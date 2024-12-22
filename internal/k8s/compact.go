package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CompactManifest holds a cleaned-up, human-readable representation of a
// Kubernetes resource with auto-generated fields removed.
type CompactManifest struct {
	Kind      string
	Name      string
	Namespace string
	Content   string // YAML-like formatted output
}

// GetConfigMapData returns the full data contents of a ConfigMap.
func (c *Client) GetConfigMapData(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting configmap %s/%s: %w", namespace, name, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# ConfigMap: %s/%s\n", namespace, name))
	sb.WriteString(fmt.Sprintf("# Keys: %d\n\n", len(cm.Data)+len(cm.BinaryData)))

	// Sort keys for stable output.
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := cm.Data[k]
		sb.WriteString(fmt.Sprintf("--- %s ---\n", k))
		sb.WriteString(v)
		if !strings.HasSuffix(v, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	// Binary data keys (show size only).
	binKeys := make([]string, 0, len(cm.BinaryData))
	for k := range cm.BinaryData {
		binKeys = append(binKeys, k)
	}
	sort.Strings(binKeys)
	for _, k := range binKeys {
		sb.WriteString(fmt.Sprintf("--- %s (binary: %d bytes) ---\n\n", k, len(cm.BinaryData[k])))
	}

	return &CompactManifest{
		Kind:      "ConfigMap",
		Name:      name,
		Namespace: namespace,
		Content:   sb.String(),
	}, nil
}

// GetSecretData returns the full data contents of a Secret with values decoded.
func (c *Client) GetSecretData(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting secret %s/%s: %w", namespace, name, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Secret: %s/%s\n", namespace, name))
	sb.WriteString(fmt.Sprintf("# Type: %s\n", secret.Type))
	sb.WriteString(fmt.Sprintf("# Keys: %d\n\n", len(secret.Data)+len(secret.StringData)))

	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := secret.Data[k]
		sb.WriteString(fmt.Sprintf("--- %s ---\n", k))
		sb.WriteString(string(v))
		if len(v) > 0 && v[len(v)-1] != '\n' {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	// StringData (less common, but possible).
	strKeys := make([]string, 0, len(secret.StringData))
	for k := range secret.StringData {
		strKeys = append(strKeys, k)
	}
	sort.Strings(strKeys)
	for _, k := range strKeys {
		sb.WriteString(fmt.Sprintf("--- %s ---\n", k))
		sb.WriteString(secret.StringData[k])
		sb.WriteByte('\n')
		sb.WriteByte('\n')
	}

	return &CompactManifest{
		Kind:      "Secret",
		Name:      name,
		Namespace: namespace,
		Content:   sb.String(),
	}, nil
}

// GetDeploymentCompact returns a compact manifest for a Deployment.
func (c *Client) GetDeploymentCompact(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	dep, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting deployment %s/%s: %w", namespace, name, err)
	}

	content := renderDeploymentCompact(dep)
	return &CompactManifest{
		Kind:      "Deployment",
		Name:      name,
		Namespace: namespace,
		Content:   content,
	}, nil
}

// GetStatefulSetCompact returns a compact manifest for a StatefulSet.
func (c *Client) GetStatefulSetCompact(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting statefulset %s/%s: %w", namespace, name, err)
	}

	content := renderStatefulSetCompact(sts)
	return &CompactManifest{
		Kind:      "StatefulSet",
		Name:      name,
		Namespace: namespace,
		Content:   content,
	}, nil
}

// GetDaemonSetCompact returns a compact manifest for a DaemonSet.
func (c *Client) GetDaemonSetCompact(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting daemonset %s/%s: %w", namespace, name, err)
	}

	content := renderDaemonSetCompact(ds)
	return &CompactManifest{
		Kind:      "DaemonSet",
		Name:      name,
		Namespace: namespace,
		Content:   content,
	}, nil
}

// GetPodCompact returns a compact manifest for a Pod.
func (c *Client) GetPodCompact(ctx context.Context, namespace, name string) (*CompactManifest, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, name, err)
	}

	content := renderPodCompact(pod)
	return &CompactManifest{
		Kind:      "Pod",
		Name:      name,
		Namespace: namespace,
		Content:   content,
	}, nil
}

// ListStatefulSets returns statefulset info for the given namespace.
func (c *Client) ListStatefulSets(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	stsList, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing statefulsets: %w", err)
	}

	var resources []ResourceInfo
	for _, s := range stsList.Items {
		replicas := int32(1)
		if s.Spec.Replicas != nil {
			replicas = *s.Spec.Replicas
		}
		resources = append(resources, ResourceInfo{
			Kind:      "StatefulSet",
			Name:      s.Name,
			Namespace: s.Namespace,
			Age:       time.Since(s.CreationTimestamp.Time),
			Status:    fmt.Sprintf("%d/%d ready", s.Status.ReadyReplicas, replicas),
			Extra: map[string]string{
				"images": extractDeploymentImages(s.Spec.Template.Spec.Containers),
			},
		})
	}
	return resources, nil
}

// ListDaemonSets returns daemonset info for the given namespace.
func (c *Client) ListDaemonSets(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	dsList, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing daemonsets: %w", err)
	}

	var resources []ResourceInfo
	for _, d := range dsList.Items {
		resources = append(resources, ResourceInfo{
			Kind:      "DaemonSet",
			Name:      d.Name,
			Namespace: d.Namespace,
			Age:       time.Since(d.CreationTimestamp.Time),
			Status:    fmt.Sprintf("%d desired, %d ready", d.Status.DesiredNumberScheduled, d.Status.NumberReady),
			Extra: map[string]string{
				"images": extractDeploymentImages(d.Spec.Template.Spec.Containers),
			},
		})
	}
	return resources, nil
}

// --- Compact renderers ---
// These produce a human-readable, YAML-like output that strips:
// - managedFields
// - resourceVersion, uid, selfLink, generation
// - creationTimestamp (shown in header instead)
// - status (shown separately where useful)
// - last-applied-configuration annotation
// - empty/nil fields

func renderDeploymentCompact(dep *appsv1.Deployment) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Deployment: %s/%s\n", dep.Namespace, dep.Name))
	sb.WriteString(fmt.Sprintf("# Created: %s\n", dep.CreationTimestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("# Status: %d/%d ready, %d updated, %d available\n\n",
		dep.Status.ReadyReplicas, ptrInt32(dep.Spec.Replicas),
		dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas))

	renderMeta(&sb, dep.ObjectMeta)

	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  replicas: %d\n", ptrInt32(dep.Spec.Replicas)))
	renderSelector(&sb, dep.Spec.Selector)
	renderStrategy(&sb, dep.Spec.Strategy.Type, dep.Spec.Strategy.RollingUpdate)
	renderPodTemplate(&sb, dep.Spec.Template)

	return sb.String()
}

func renderStatefulSetCompact(sts *appsv1.StatefulSet) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# StatefulSet: %s/%s\n", sts.Namespace, sts.Name))
	sb.WriteString(fmt.Sprintf("# Created: %s\n", sts.CreationTimestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("# Status: %d/%d ready\n\n",
		sts.Status.ReadyReplicas, ptrInt32(sts.Spec.Replicas)))

	renderMeta(&sb, sts.ObjectMeta)

	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  replicas: %d\n", ptrInt32(sts.Spec.Replicas)))
	sb.WriteString(fmt.Sprintf("  serviceName: %s\n", sts.Spec.ServiceName))
	if sts.Spec.PodManagementPolicy != "" {
		sb.WriteString(fmt.Sprintf("  podManagementPolicy: %s\n", sts.Spec.PodManagementPolicy))
	}
	renderSelector(&sb, sts.Spec.Selector)
	renderPodTemplate(&sb, sts.Spec.Template)

	// VolumeClaimTemplates.
	if len(sts.Spec.VolumeClaimTemplates) > 0 {
		sb.WriteString("  volumeClaimTemplates:\n")
		for _, pvc := range sts.Spec.VolumeClaimTemplates {
			sb.WriteString(fmt.Sprintf("    - name: %s\n", pvc.Name))
			if len(pvc.Spec.AccessModes) > 0 {
				modes := make([]string, len(pvc.Spec.AccessModes))
				for i, m := range pvc.Spec.AccessModes {
					modes[i] = string(m)
				}
				sb.WriteString(fmt.Sprintf("      accessModes: [%s]\n", strings.Join(modes, ", ")))
			}
			if pvc.Spec.Resources.Requests != nil {
				if storage, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
					sb.WriteString(fmt.Sprintf("      storage: %s\n", storage.String()))
				}
			}
			if pvc.Spec.StorageClassName != nil {
				sb.WriteString(fmt.Sprintf("      storageClassName: %s\n", *pvc.Spec.StorageClassName))
			}
		}
	}

	return sb.String()
}

func renderDaemonSetCompact(ds *appsv1.DaemonSet) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# DaemonSet: %s/%s\n", ds.Namespace, ds.Name))
	sb.WriteString(fmt.Sprintf("# Created: %s\n", ds.CreationTimestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("# Status: %d desired, %d ready, %d updated\n\n",
		ds.Status.DesiredNumberScheduled, ds.Status.NumberReady, ds.Status.UpdatedNumberScheduled))

	renderMeta(&sb, ds.ObjectMeta)

	sb.WriteString("spec:\n")
	renderSelector(&sb, ds.Spec.Selector)
	if ds.Spec.UpdateStrategy.Type != "" {
		sb.WriteString(fmt.Sprintf("  updateStrategy: %s\n", ds.Spec.UpdateStrategy.Type))
	}
	renderPodTemplate(&sb, ds.Spec.Template)

	return sb.String()
}

func renderPodCompact(pod *corev1.Pod) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Pod: %s/%s\n", pod.Namespace, pod.Name))
	sb.WriteString(fmt.Sprintf("# Created: %s\n", pod.CreationTimestamp.Format("2006-01-02 15:04:05 UTC")))
	sb.WriteString(fmt.Sprintf("# Phase: %s  Node: %s  IP: %s\n\n",
		pod.Status.Phase, pod.Spec.NodeName, pod.Status.PodIP))

	renderMeta(&sb, pod.ObjectMeta)

	sb.WriteString("spec:\n")
	if pod.Spec.ServiceAccountName != "" {
		sb.WriteString(fmt.Sprintf("  serviceAccountName: %s\n", pod.Spec.ServiceAccountName))
	}
	if pod.Spec.RestartPolicy != "" {
		sb.WriteString(fmt.Sprintf("  restartPolicy: %s\n", pod.Spec.RestartPolicy))
	}
	if pod.Spec.PriorityClassName != "" {
		sb.WriteString(fmt.Sprintf("  priorityClassName: %s\n", pod.Spec.PriorityClassName))
	}
	renderNodeScheduling(&sb, pod.Spec)

	if len(pod.Spec.InitContainers) > 0 {
		sb.WriteString("  initContainers:\n")
		for _, c := range pod.Spec.InitContainers {
			renderContainer(&sb, c, "    ")
		}
	}

	sb.WriteString("  containers:\n")
	for _, c := range pod.Spec.Containers {
		renderContainer(&sb, c, "    ")
	}

	renderVolumes(&sb, pod.Spec.Volumes)

	// Compact status for running containers.
	sb.WriteString("\n# Container Status:\n")
	for _, cs := range pod.Status.ContainerStatuses {
		state := "unknown"
		if cs.State.Running != nil {
			state = "running"
		} else if cs.State.Waiting != nil {
			state = "waiting: " + cs.State.Waiting.Reason
		} else if cs.State.Terminated != nil {
			state = "terminated: " + cs.State.Terminated.Reason
		}
		sb.WriteString(fmt.Sprintf("#   %s: ready=%v restarts=%d state=%s\n",
			cs.Name, cs.Ready, cs.RestartCount, state))
	}

	return sb.String()
}

// --- Shared rendering helpers ---

func renderMeta(sb *strings.Builder, meta metav1.ObjectMeta) {
	// Labels.
	if len(meta.Labels) > 0 {
		sb.WriteString("labels:\n")
		keys := sortedKeys(meta.Labels)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, meta.Labels[k]))
		}
	}

	// Annotations — filter out noise.
	cleaned := cleanAnnotations(meta.Annotations)
	if len(cleaned) > 0 {
		sb.WriteString("annotations:\n")
		keys := sortedKeys(cleaned)
		for _, k := range keys {
			v := cleaned[k]
			// Truncate very long annotation values.
			if len(v) > 200 {
				v = v[:200] + "... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}
}

func renderSelector(sb *strings.Builder, sel *metav1.LabelSelector) {
	if sel == nil {
		return
	}
	if len(sel.MatchLabels) > 0 {
		sb.WriteString("  selector:\n")
		for k, v := range sel.MatchLabels {
			sb.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
		}
	}
}

func renderStrategy(sb *strings.Builder, sType appsv1.DeploymentStrategyType, rolling *appsv1.RollingUpdateDeployment) {
	if sType == "" {
		return
	}
	sb.WriteString(fmt.Sprintf("  strategy: %s\n", sType))
	if rolling != nil {
		if rolling.MaxUnavailable != nil {
			sb.WriteString(fmt.Sprintf("    maxUnavailable: %s\n", rolling.MaxUnavailable.String()))
		}
		if rolling.MaxSurge != nil {
			sb.WriteString(fmt.Sprintf("    maxSurge: %s\n", rolling.MaxSurge.String()))
		}
	}
}

func renderPodTemplate(sb *strings.Builder, tmpl corev1.PodTemplateSpec) {
	sb.WriteString("  template:\n")

	// Template labels.
	if len(tmpl.Labels) > 0 {
		sb.WriteString("    labels:\n")
		for k, v := range tmpl.Labels {
			sb.WriteString(fmt.Sprintf("      %s: %s\n", k, v))
		}
	}

	// Template annotations (cleaned).
	cleaned := cleanAnnotations(tmpl.Annotations)
	if len(cleaned) > 0 {
		sb.WriteString("    annotations:\n")
		for k, v := range cleaned {
			if len(v) > 200 {
				v = v[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("      %s: %s\n", k, v))
		}
	}

	spec := tmpl.Spec

	if spec.ServiceAccountName != "" {
		sb.WriteString(fmt.Sprintf("    serviceAccountName: %s\n", spec.ServiceAccountName))
	}
	renderNodeScheduling(sb, spec)

	if len(spec.InitContainers) > 0 {
		sb.WriteString("    initContainers:\n")
		for _, c := range spec.InitContainers {
			renderContainer(sb, c, "      ")
		}
	}

	sb.WriteString("    containers:\n")
	for _, c := range spec.Containers {
		renderContainer(sb, c, "      ")
	}

	renderVolumes(sb, spec.Volumes)
}

func renderContainer(sb *strings.Builder, c corev1.Container, indent string) {
	sb.WriteString(fmt.Sprintf("%s- name: %s\n", indent, c.Name))
	sb.WriteString(fmt.Sprintf("%s  image: %s\n", indent, c.Image))

	if c.ImagePullPolicy != "" && c.ImagePullPolicy != corev1.PullIfNotPresent {
		sb.WriteString(fmt.Sprintf("%s  imagePullPolicy: %s\n", indent, c.ImagePullPolicy))
	}

	if len(c.Command) > 0 {
		sb.WriteString(fmt.Sprintf("%s  command: %s\n", indent, formatStringSlice(c.Command)))
	}
	if len(c.Args) > 0 {
		sb.WriteString(fmt.Sprintf("%s  args: %s\n", indent, formatStringSlice(c.Args)))
	}

	// Ports.
	if len(c.Ports) > 0 {
		sb.WriteString(fmt.Sprintf("%s  ports:\n", indent))
		for _, p := range c.Ports {
			line := fmt.Sprintf("%s    - %d/%s", indent, p.ContainerPort, p.Protocol)
			if p.Name != "" {
				line += fmt.Sprintf(" (%s)", p.Name)
			}
			sb.WriteString(line + "\n")
		}
	}

	// Env (inline).
	if len(c.Env) > 0 {
		sb.WriteString(fmt.Sprintf("%s  env:\n", indent))
		for _, e := range c.Env {
			if e.Value != "" {
				sb.WriteString(fmt.Sprintf("%s    %s: %s\n", indent, e.Name, e.Value))
			} else if e.ValueFrom != nil {
				sb.WriteString(fmt.Sprintf("%s    %s: <%s>\n", indent, e.Name, describeValueSource(e.ValueFrom)))
			}
		}
	}

	// EnvFrom.
	if len(c.EnvFrom) > 0 {
		sb.WriteString(fmt.Sprintf("%s  envFrom:\n", indent))
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				line := fmt.Sprintf("%s    - configMapRef: %s", indent, ef.ConfigMapRef.Name)
				if ef.Prefix != "" {
					line += fmt.Sprintf(" (prefix: %s)", ef.Prefix)
				}
				sb.WriteString(line + "\n")
			}
			if ef.SecretRef != nil {
				line := fmt.Sprintf("%s    - secretRef: %s", indent, ef.SecretRef.Name)
				if ef.Prefix != "" {
					line += fmt.Sprintf(" (prefix: %s)", ef.Prefix)
				}
				sb.WriteString(line + "\n")
			}
		}
	}

	// Resources.
	if c.Resources.Requests != nil || c.Resources.Limits != nil {
		sb.WriteString(fmt.Sprintf("%s  resources:\n", indent))
		if c.Resources.Requests != nil {
			sb.WriteString(fmt.Sprintf("%s    requests:", indent))
			for k, v := range c.Resources.Requests {
				sb.WriteString(fmt.Sprintf(" %s=%s", k, v.String()))
			}
			sb.WriteByte('\n')
		}
		if c.Resources.Limits != nil {
			sb.WriteString(fmt.Sprintf("%s    limits:", indent))
			for k, v := range c.Resources.Limits {
				sb.WriteString(fmt.Sprintf(" %s=%s", k, v.String()))
			}
			sb.WriteByte('\n')
		}
	}

	// Probes.
	renderProbe(sb, "livenessProbe", c.LivenessProbe, indent)
	renderProbe(sb, "readinessProbe", c.ReadinessProbe, indent)
	renderProbe(sb, "startupProbe", c.StartupProbe, indent)

	// Volume mounts.
	if len(c.VolumeMounts) > 0 {
		sb.WriteString(fmt.Sprintf("%s  volumeMounts:\n", indent))
		for _, vm := range c.VolumeMounts {
			line := fmt.Sprintf("%s    - %s → %s", indent, vm.Name, vm.MountPath)
			if vm.ReadOnly {
				line += " (ro)"
			}
			if vm.SubPath != "" {
				line += fmt.Sprintf(" subPath=%s", vm.SubPath)
			}
			sb.WriteString(line + "\n")
		}
	}

	// Security context.
	if sc := c.SecurityContext; sc != nil {
		parts := []string{}
		if sc.RunAsUser != nil {
			parts = append(parts, fmt.Sprintf("runAsUser=%d", *sc.RunAsUser))
		}
		if sc.RunAsNonRoot != nil && *sc.RunAsNonRoot {
			parts = append(parts, "runAsNonRoot")
		}
		if sc.ReadOnlyRootFilesystem != nil && *sc.ReadOnlyRootFilesystem {
			parts = append(parts, "readOnlyRootFs")
		}
		if sc.Privileged != nil && *sc.Privileged {
			parts = append(parts, "privileged")
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("%s  securityContext: %s\n", indent, strings.Join(parts, ", ")))
		}
	}
}

func renderProbe(sb *strings.Builder, name string, probe *corev1.Probe, indent string) {
	if probe == nil {
		return
	}
	sb.WriteString(fmt.Sprintf("%s  %s:", indent, name))

	if probe.HTTPGet != nil {
		sb.WriteString(fmt.Sprintf(" httpGet %s:%v%s",
			probe.HTTPGet.Scheme, probe.HTTPGet.Port.IntValue(), probe.HTTPGet.Path))
	} else if probe.TCPSocket != nil {
		sb.WriteString(fmt.Sprintf(" tcpSocket :%v", probe.TCPSocket.Port.IntValue()))
	} else if probe.Exec != nil {
		sb.WriteString(fmt.Sprintf(" exec %s", formatStringSlice(probe.Exec.Command)))
	}

	details := []string{}
	if probe.InitialDelaySeconds > 0 {
		details = append(details, fmt.Sprintf("delay=%ds", probe.InitialDelaySeconds))
	}
	if probe.PeriodSeconds > 0 {
		details = append(details, fmt.Sprintf("period=%ds", probe.PeriodSeconds))
	}
	if probe.TimeoutSeconds > 0 && probe.TimeoutSeconds != 1 {
		details = append(details, fmt.Sprintf("timeout=%ds", probe.TimeoutSeconds))
	}
	if len(details) > 0 {
		sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(details, ", ")))
	}
	sb.WriteByte('\n')
}

func renderVolumes(sb *strings.Builder, volumes []corev1.Volume) {
	if len(volumes) == 0 {
		return
	}

	// Filter out default service account token volumes.
	var filtered []corev1.Volume
	for _, v := range volumes {
		if strings.HasPrefix(v.Name, "kube-api-access-") {
			continue
		}
		if v.Projected != nil && len(v.Projected.Sources) > 0 {
			allSA := true
			for _, src := range v.Projected.Sources {
				if src.ServiceAccountToken == nil && src.ConfigMap == nil && src.DownwardAPI == nil {
					allSA = false
					break
				}
			}
			if allSA && strings.HasPrefix(v.Name, "kube-api-access") {
				continue
			}
		}
		filtered = append(filtered, v)
	}

	if len(filtered) == 0 {
		return
	}

	sb.WriteString("  volumes:\n")
	for _, v := range filtered {
		sb.WriteString(fmt.Sprintf("    - name: %s\n", v.Name))
		switch {
		case v.ConfigMap != nil:
			sb.WriteString(fmt.Sprintf("      configMap: %s\n", v.ConfigMap.Name))
		case v.Secret != nil:
			sb.WriteString(fmt.Sprintf("      secret: %s\n", v.Secret.SecretName))
		case v.PersistentVolumeClaim != nil:
			sb.WriteString(fmt.Sprintf("      pvc: %s\n", v.PersistentVolumeClaim.ClaimName))
		case v.EmptyDir != nil:
			sb.WriteString("      emptyDir: {}\n")
		case v.HostPath != nil:
			sb.WriteString(fmt.Sprintf("      hostPath: %s\n", v.HostPath.Path))
		case v.DownwardAPI != nil:
			sb.WriteString("      downwardAPI: (field refs)\n")
		default:
			// Best-effort: marshal to JSON for unknown types.
			data, err := json.Marshal(v.VolumeSource)
			if err == nil {
				sb.WriteString(fmt.Sprintf("      source: %s\n", string(data)))
			}
		}
	}
}

func renderNodeScheduling(sb *strings.Builder, spec corev1.PodSpec) {
	if spec.NodeSelector != nil {
		sb.WriteString("    nodeSelector:\n")
		for k, v := range spec.NodeSelector {
			sb.WriteString(fmt.Sprintf("      %s: %s\n", k, v))
		}
	}

	if len(spec.Tolerations) > 0 {
		// Filter default tolerations.
		var real []corev1.Toleration
		for _, t := range spec.Tolerations {
			if t.Key == "node.kubernetes.io/not-ready" || t.Key == "node.kubernetes.io/unreachable" {
				continue
			}
			real = append(real, t)
		}
		if len(real) > 0 {
			sb.WriteString("    tolerations:\n")
			for _, t := range real {
				sb.WriteString(fmt.Sprintf("      - %s %s %s", t.Key, t.Operator, t.Effect))
				if t.Value != "" {
					sb.WriteString(fmt.Sprintf("=%s", t.Value))
				}
				sb.WriteByte('\n')
			}
		}
	}

	if spec.Affinity != nil {
		sb.WriteString("    affinity: (defined)\n")
	}
}

// --- Utility ---

// cleanAnnotations removes auto-generated annotations that add noise.
func cleanAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return nil
	}

	noise := map[string]bool{
		"kubectl.kubernetes.io/last-applied-configuration": true,
		"deployment.kubernetes.io/revision":                true,
		"kubernetes.io/change-cause":                       true,
	}
	noisePrefix := []string{
		"control-plane.alpha.kubernetes.io/",
		"deprecated.daemonset.template.generation",
	}

	result := make(map[string]string, len(annotations))
	for k, v := range annotations {
		if noise[k] {
			continue
		}
		skip := false
		for _, p := range noisePrefix {
			if strings.HasPrefix(k, p) {
				skip = true
				break
			}
		}
		if !skip {
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatStringSlice(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		if strings.ContainsAny(s, " \t\"'") {
			quoted[i] = fmt.Sprintf("%q", s)
		} else {
			quoted[i] = s
		}
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func ptrInt32(p *int32) int32 {
	if p == nil {
		return 1
	}
	return *p
}
