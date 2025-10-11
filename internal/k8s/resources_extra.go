package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

// ListJobs returns Job info for the given namespace.
func (c *Client) ListJobs(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, j := range list.Items {
		resources = append(resources, ResourceInfo{
			Kind:      "Job",
			Name:      j.Name,
			Namespace: j.Namespace,
			Age:       time.Since(j.CreationTimestamp.Time),
			Status:    jobStatus(j),
			Extra: map[string]string{
				"completions": fmt.Sprintf("%d/%d", j.Status.Succeeded, jobCompletions(j)),
				"active":      fmt.Sprintf("%d", j.Status.Active),
				"failed":      fmt.Sprintf("%d", j.Status.Failed),
			},
		})
	}
	return resources, nil
}

// ListCronJobs returns CronJob info for the given namespace.
func (c *Client) ListCronJobs(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing cronjobs: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, cj := range list.Items {
		status := "Active"
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			status = "Suspended"
		}
		extra := map[string]string{
			"schedule": cj.Spec.Schedule,
			"active":   fmt.Sprintf("%d", len(cj.Status.Active)),
		}
		if cj.Status.LastScheduleTime != nil {
			extra["lastSchedule"] = cj.Status.LastScheduleTime.Format(time.RFC3339)
		}
		resources = append(resources, ResourceInfo{
			Kind:      "CronJob",
			Name:      cj.Name,
			Namespace: cj.Namespace,
			Age:       time.Since(cj.CreationTimestamp.Time),
			Status:    status,
			Extra:     extra,
		})
	}
	return resources, nil
}

// ListIngresses returns Ingress info (networking.k8s.io/v1) for the given namespace.
func (c *Client) ListIngresses(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ingresses: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, ing := range list.Items {
		hosts := make([]string, 0, len(ing.Spec.Rules))
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				hosts = append(hosts, rule.Host)
			}
		}
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		resources = append(resources, ResourceInfo{
			Kind:      "Ingress",
			Name:      ing.Name,
			Namespace: ing.Namespace,
			Age:       time.Since(ing.CreationTimestamp.Time),
			Status:    ingressAddress(ing.Status.LoadBalancer),
			Extra: map[string]string{
				"hosts": strings.Join(hosts, ", "),
				"class": class,
			},
		})
	}
	return resources, nil
}

// ListPersistentVolumeClaims returns PVC info for the given namespace.
func (c *Client) ListPersistentVolumeClaims(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing persistentvolumeclaims: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, pvc := range list.Items {
		storage := ""
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			storage = q.String()
		}
		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		resources = append(resources, ResourceInfo{
			Kind:      "PersistentVolumeClaim",
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Age:       time.Since(pvc.CreationTimestamp.Time),
			Status:    string(pvc.Status.Phase),
			Extra: map[string]string{
				"capacity":     storage,
				"storageClass": sc,
				"volume":       pvc.Spec.VolumeName,
				"accessModes":  accessModesString(pvc.Spec.AccessModes),
			},
		})
	}
	return resources, nil
}

// ListHPAs returns HorizontalPodAutoscaler info (autoscaling/v2) for the given namespace.
func (c *Client) ListHPAs(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing hpas: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, hpa := range list.Items {
		minReplicas := int32(1)
		if hpa.Spec.MinReplicas != nil {
			minReplicas = *hpa.Spec.MinReplicas
		}
		ref := fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name)
		resources = append(resources, ResourceInfo{
			Kind:      "HorizontalPodAutoscaler",
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
			Age:       time.Since(hpa.CreationTimestamp.Time),
			Status:    fmt.Sprintf("%d/%d", hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas),
			Extra: map[string]string{
				"min":    fmt.Sprintf("%d", minReplicas),
				"max":    fmt.Sprintf("%d", hpa.Spec.MaxReplicas),
				"target": ref,
			},
		})
	}
	return resources, nil
}

// ListCRDs returns cluster-scoped CustomResourceDefinition names.
// Requires rest.Config; returns an empty list when config is nil (e.g. fake tests).
func (c *Client) ListCRDs(ctx context.Context) ([]ResourceInfo, error) {
	if c.config == nil {
		return nil, nil
	}

	ext, err := apiextensionsclientset.NewForConfig(c.config)
	if err != nil {
		return nil, fmt.Errorf("creating apiextensions client: %w", err)
	}

	list, err := ext.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing crds: %w", err)
	}

	resources := make([]ResourceInfo, 0, len(list.Items))
	for _, crd := range list.Items {
		version := ""
		if len(crd.Spec.Versions) > 0 {
			for _, v := range crd.Spec.Versions {
				if v.Storage {
					version = v.Name
					break
				}
			}
			if version == "" {
				version = crd.Spec.Versions[0].Name
			}
		}
		scope := string(crd.Spec.Scope)
		resources = append(resources, ResourceInfo{
			Kind:      "CustomResourceDefinition",
			Name:      crd.Name,
			Namespace: "",
			Age:       time.Since(crd.CreationTimestamp.Time),
			Status:    scope,
			Extra: map[string]string{
				"group":   crd.Spec.Group,
				"version": version,
				"kind":    crd.Spec.Names.Kind,
				"scope":   scope,
			},
		})
	}
	return resources, nil
}

// ListHelmReleases lists Helm releases by scanning Secrets with owner=helm
// and type helm.sh/release.v1. Keeps the latest revision per release name.
func (c *Client) ListHelmReleases(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	list, err := c.clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		return nil, fmt.Errorf("listing helm release secrets: %w", err)
	}

	type releaseRev struct {
		info    ResourceInfo
		version int
	}
	latest := make(map[string]releaseRev)

	for _, s := range list.Items {
		if s.Type != "helm.sh/release.v1" {
			continue
		}

		name := s.Labels["name"]
		versionStr := s.Labels["version"]
		status := s.Labels["status"]

		if name == "" || versionStr == "" {
			parsedName, parsedVer, ok := parseHelmSecretName(s.Name)
			if !ok {
				continue
			}
			if name == "" {
				name = parsedName
			}
			if versionStr == "" {
				versionStr = strconv.Itoa(parsedVer)
			}
		}

		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}
		if status == "" {
			status = "unknown"
		}

		info := ResourceInfo{
			Kind:      "HelmRelease",
			Name:      name,
			Namespace: s.Namespace,
			Age:       time.Since(s.CreationTimestamp.Time),
			Status:    status,
			Extra: map[string]string{
				"version":    versionStr,
				"revision":   versionStr,
				"secretName": s.Name,
			},
		}

		if prev, ok := latest[name]; !ok || version > prev.version {
			latest[name] = releaseRev{info: info, version: version}
		}
	}

	resources := make([]ResourceInfo, 0, len(latest))
	for _, rev := range latest {
		resources = append(resources, rev.info)
	}
	return resources, nil
}

func jobCompletions(j batchv1.Job) int32 {
	if j.Spec.Completions != nil {
		return *j.Spec.Completions
	}
	return 1
}

func jobStatus(j batchv1.Job) string {
	for _, cond := range j.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		switch cond.Type {
		case batchv1.JobComplete:
			return "Complete"
		case batchv1.JobFailed:
			return "Failed"
		}
	}
	if j.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}

func ingressAddress(lb networkingv1.IngressLoadBalancerStatus) string {
	if len(lb.Ingress) == 0 {
		return "Pending"
	}
	parts := make([]string, 0, len(lb.Ingress))
	for _, ing := range lb.Ingress {
		if ing.IP != "" {
			parts = append(parts, ing.IP)
		} else if ing.Hostname != "" {
			parts = append(parts, ing.Hostname)
		}
	}
	if len(parts) == 0 {
		return "Pending"
	}
	return strings.Join(parts, ", ")
}

func accessModesString(modes []corev1.PersistentVolumeAccessMode) string {
	parts := make([]string, len(modes))
	for i, m := range modes {
		parts[i] = string(m)
	}
	return strings.Join(parts, ",")
}

// parseHelmSecretName extracts release name and revision from
// sh.helm.release.v1.<release>.v<version>.
func parseHelmSecretName(secretName string) (name string, version int, ok bool) {
	const prefix = "sh.helm.release.v1."
	if !strings.HasPrefix(secretName, prefix) {
		return "", 0, false
	}
	rest := strings.TrimPrefix(secretName, prefix)
	idx := strings.LastIndex(rest, ".v")
	if idx < 0 {
		return "", 0, false
	}
	name = rest[:idx]
	verStr := rest[idx+2:]
	version, err := strconv.Atoi(verStr)
	if err != nil || name == "" {
		return "", 0, false
	}
	return name, version, true
}
