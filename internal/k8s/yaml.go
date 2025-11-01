package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/yaml"
)

// LintIssue describes a YAML or Kubernetes validation problem.
type LintIssue struct {
	Level   string `json:"level"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

var kindToGVR = map[string]schema.GroupVersionResource{
	"Pod":                      {Group: "", Version: "v1", Resource: "pods"},
	"Namespace":                {Group: "", Version: "v1", Resource: "namespaces"},
	"Node":                     {Group: "", Version: "v1", Resource: "nodes"},
	"Service":                  {Group: "", Version: "v1", Resource: "services"},
	"ConfigMap":                {Group: "", Version: "v1", Resource: "configmaps"},
	"Secret":                   {Group: "", Version: "v1", Resource: "secrets"},
	"PersistentVolume":         {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"PersistentVolumeClaim":    {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
	"Event":                    {Group: "", Version: "v1", Resource: "events"},
	"Deployment":               {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet":              {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":                {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"Job":                      {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":                  {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"Ingress":                  {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"HorizontalPodAutoscaler":  {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
	"CustomResourceDefinition": {Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
}

var clusterScopedKinds = map[string]bool{
	"Namespace":                true,
	"Node":                     true,
	"PersistentVolume":         true,
	"CustomResourceDefinition": true,
}

func (c *Client) dynamicOrErr() (dynamic.Interface, error) {
	if c.config == nil {
		return nil, fmt.Errorf("kubernetes rest config is unavailable")
	}
	return dynamic.NewForConfig(c.config)
}

// GetResourceYAML returns the live object as YAML suitable for editing.
func (c *Client) GetResourceYAML(ctx context.Context, kind, namespace, name string) (string, error) {
	gvr, ok := kindToGVR[kind]
	if !ok {
		return "", fmt.Errorf("unsupported yaml kind %q", kind)
	}
	dyn, err := c.dynamicOrErr()
	if err != nil {
		return "", err
	}

	var obj *unstructured.Unstructured
	if clusterScopedKinds[kind] || namespace == "" {
		obj, err = dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return "", fmt.Errorf("getting %s/%s: %w", kind, name, err)
	}

	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "status")

	out, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", fmt.Errorf("encoding yaml: %w", err)
	}
	return string(out), nil
}

// LintYAML parses YAML and optionally dry-runs a server-side apply.
func (c *Client) LintYAML(ctx context.Context, manifest string, dryRun bool) ([]LintIssue, error) {
	issues := LintYAMLClient(manifest)
	if len(issues) > 0 || !dryRun {
		return issues, nil
	}
	if _, err := c.applyYAML(ctx, manifest, true); err != nil {
		issues = append(issues, LintIssue{Level: "error", Message: err.Error()})
	}
	return issues, nil
}

// LintYAMLClient validates YAML structure without contacting the API server.
func LintYAMLClient(manifest string) []LintIssue {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return []LintIssue{{Level: "error", Message: "manifest is empty"}}
	}

	var obj map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		line := 0
		msg := err.Error()
		if loc := strings.Index(msg, "line "); loc >= 0 {
			_, _ = fmt.Sscanf(msg[loc:], "line %d", &line)
		}
		return []LintIssue{{Level: "error", Line: line, Message: msg}}
	}

	var issues []LintIssue
	if apiVersion, _ := obj["apiVersion"].(string); strings.TrimSpace(apiVersion) == "" {
		issues = append(issues, LintIssue{Level: "error", Message: "apiVersion is required"})
	}
	if kind, _ := obj["kind"].(string); strings.TrimSpace(kind) == "" {
		issues = append(issues, LintIssue{Level: "error", Message: "kind is required"})
	}
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		issues = append(issues, LintIssue{Level: "error", Message: "metadata is required"})
	} else if name, _ := meta["name"].(string); strings.TrimSpace(name) == "" {
		issues = append(issues, LintIssue{Level: "error", Message: "metadata.name is required"})
	}
	return issues
}

// ApplyYAML applies a YAML manifest via server-side apply.
func (c *Client) ApplyYAML(ctx context.Context, manifest string) (string, error) {
	return c.applyYAML(ctx, manifest, false)
}

func (c *Client) applyYAML(ctx context.Context, manifest string, dryRun bool) (string, error) {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(manifest), &obj.Object); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}
	if obj.GetName() == "" {
		return "", fmt.Errorf("metadata.name is required")
	}
	if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
		return "", fmt.Errorf("apiVersion and kind are required")
	}

	dyn, err := c.dynamicOrErr()
	if err != nil {
		return "", err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(c.clientset.Discovery()))
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return "", fmt.Errorf("resolve resource mapping for %s: %w", gvk.String(), err)
	}

	opts := metav1.ApplyOptions{FieldManager: "pyxis-cli", Force: true}
	if dryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	var applied *unstructured.Unstructured
	ns := obj.GetNamespace()
	if mapping.Scope.Name() == "namespace" {
		if ns == "" {
			return "", fmt.Errorf("namespace is required for namespaced kind %s", obj.GetKind())
		}
		applied, err = dyn.Resource(mapping.Resource).Namespace(ns).Apply(ctx, obj.GetName(), obj, opts)
	} else {
		applied, err = dyn.Resource(mapping.Resource).Apply(ctx, obj.GetName(), obj, opts)
	}
	if err != nil {
		return "", err
	}
	action := "Applied"
	if dryRun {
		action = "Dry-run ok"
	}
	if ns != "" {
		return fmt.Sprintf("%s %s %s/%s", action, applied.GetKind(), ns, applied.GetName()), nil
	}
	return fmt.Sprintf("%s %s %s", action, applied.GetKind(), applied.GetName()), nil
}

// ListPersistentVolumes returns cluster-scoped PersistentVolume info.
func (c *Client) ListPersistentVolumes(ctx context.Context) ([]ResourceInfo, error) {
	list, err := c.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing persistentvolumes: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, pv := range list.Items {
		capacity := ""
		if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			capacity = q.String()
		}
		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		resources = append(resources, ResourceInfo{
			Kind:      "PersistentVolume",
			Name:      pv.Name,
			Namespace: "",
			Age:       time.Since(pv.CreationTimestamp.Time),
			Status:    string(pv.Status.Phase),
			Extra: map[string]string{
				"capacity":     capacity,
				"storageClass": pv.Spec.StorageClassName,
				"claim":        claim,
				"accessModes":  accessModesString(pv.Spec.AccessModes),
				"reclaim":      string(pv.Spec.PersistentVolumeReclaimPolicy),
			},
		})
	}
	return resources, nil
}

// ListNamespaceInfos returns Namespace resources for the manager view.
func (c *Client) ListNamespaceInfos(ctx context.Context) ([]ResourceInfo, error) {
	list, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, ns := range list.Items {
		resources = append(resources, ResourceInfo{
			Kind:      "Namespace",
			Name:      ns.Name,
			Namespace: "",
			Age:       time.Since(ns.CreationTimestamp.Time),
			Status:    string(ns.Status.Phase),
			Extra: map[string]string{
				"labels": labelsSummary(ns.Labels),
			},
		})
	}
	return resources, nil
}

// CreateNamespace creates a namespace with the given name.
func (c *Client) CreateNamespace(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("namespace name is required")
	}
	_, err := c.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating namespace %q: %w", name, err)
	}
	return nil
}

// DeleteNamespace deletes a namespace by name.
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("namespace name is required")
	}
	err := c.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("namespace %q not found", name)
		}
		return fmt.Errorf("deleting namespace %q: %w", name, err)
	}
	return nil
}

func labelsSummary(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
		if len(parts) >= 4 {
			parts = append(parts, "…")
			break
		}
	}
	return strings.Join(parts, ", ")
}
