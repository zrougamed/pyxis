// Package k8s provides a high-level Kubernetes client for common DevOps operations.
// It uses client-go for API access and supports kubeconfig-based context switching.
package k8s

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// PodPhaseFilter determines which pods to include based on phase.
type PodPhaseFilter int

const (
	PodFilterAll PodPhaseFilter = iota
	PodFilterRunning
	PodFilterNotRunning
	PodFilterFailed
	PodFilterPending
	PodFilterSucceeded
)

// PodInfo holds summarised pod information.
type PodInfo struct {
	Name       string
	Namespace  string
	Phase      corev1.PodPhase
	Ready      string
	Restarts   int32
	Age        time.Duration
	Node       string
	Images     []string
	Labels     map[string]string
	Containers []ContainerInfo

	// Metrics (metrics-server + kubelet stats/summary).
	CPUMillicores  int64    `json:"cpuMillicores,omitempty"`
	MemoryBytes    int64    `json:"memoryBytes,omitempty"`
	DiskBytes      int64    `json:"diskBytes,omitempty"`
	NetworkRxBytes int64    `json:"networkRxBytes,omitempty"`
	NetworkTxBytes int64    `json:"networkTxBytes,omitempty"`
	CPULabel       string   `json:"cpuLabel,omitempty"`
	MemoryLabel    string   `json:"memoryLabel,omitempty"`
	DiskLabel      string   `json:"diskLabel,omitempty"`
	NetworkLabel   string   `json:"networkLabel,omitempty"`
	CPUPercent     *float64 `json:"cpuPercent,omitempty"`
	MemoryPercent  *float64 `json:"memoryPercent,omitempty"`
	DiskPercent    *float64 `json:"diskPercent,omitempty"`
	NetworkPercent *float64 `json:"networkPercent,omitempty"`
}

// ContainerInfo holds container-level details.
type ContainerInfo struct {
	Name    string
	Image   string
	Ready   bool
	State   string
	EnvVars []EnvVar
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name      string
	Value     string
	ValueFrom string // describes the source if not a plain value
}

// ResourceInfo holds generic resource information for display.
type ResourceInfo struct {
	Kind      string
	Name      string
	Namespace string
	Age       time.Duration
	Status    string
	Extra     map[string]string

	// Metrics (metrics-server + kubelet stats/summary).
	CPUMillicores  int64    `json:"cpuMillicores,omitempty"`
	MemoryBytes    int64    `json:"memoryBytes,omitempty"`
	DiskBytes      int64    `json:"diskBytes,omitempty"`
	NetworkRxBytes int64    `json:"networkRxBytes,omitempty"`
	NetworkTxBytes int64    `json:"networkTxBytes,omitempty"`
	CPULabel       string   `json:"cpuLabel,omitempty"`
	MemoryLabel    string   `json:"memoryLabel,omitempty"`
	DiskLabel      string   `json:"diskLabel,omitempty"`
	NetworkLabel   string   `json:"networkLabel,omitempty"`
	CPUPercent     *float64 `json:"cpuPercent,omitempty"`
	MemoryPercent  *float64 `json:"memoryPercent,omitempty"`
	DiskPercent    *float64 `json:"diskPercent,omitempty"`
	NetworkPercent *float64 `json:"networkPercent,omitempty"`
}

// Client wraps the Kubernetes clientset and configuration.
type Client struct {
	clientset     kubernetes.Interface
	metricsClient metricsclientset.Interface
	config        *rest.Config
	rawConfig     api.Config
	kubeconfig    string
	context       string
}

// DefaultKubeconfig returns the default kubeconfig path.
func DefaultKubeconfig() string {
	if home := homedir.HomeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}
	return ""
}

// NewClient creates a new Kubernetes client from the given kubeconfig and context.
// If kubeconfig is empty, it uses the default path.
// If ctx is empty, it uses the current context.
func NewClient(kubeconfig, ctx string) (*Client, error) {
	if kubeconfig == "" {
		kubeconfig = DefaultKubeconfig()
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	overrides := &clientcmd.ConfigOverrides{}
	if ctx != "" {
		overrides.CurrentContext = ctx
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building config: %w", err)
	}

	// Set reasonable defaults for QPS and burst.
	config.QPS = 100
	config.Burst = 200

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}

	// metrics-server is optional; keep going if the client cannot be built.
	var metricsClient metricsclientset.Interface
	if mc, mErr := metricsclientset.NewForConfig(config); mErr == nil {
		metricsClient = mc
	}

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("loading raw config: %w", err)
	}

	return &Client{
		clientset:     clientset,
		metricsClient: metricsClient,
		config:        config,
		rawConfig:     rawConfig,
		kubeconfig:    kubeconfig,
		context:       rawConfig.CurrentContext,
	}, nil
}

// NewClientWithInterface creates a Client with a pre-built clientset (for testing).
func NewClientWithInterface(cs kubernetes.Interface) *Client {
	return &Client{
		clientset: cs,
		context:   "test-context",
	}
}

// CurrentContext returns the active context name.
func (c *Client) CurrentContext() string {
	return c.context
}

// Contexts returns all available context names.
func (c *Client) Contexts() []string {
	names := make([]string, 0, len(c.rawConfig.Contexts))
	for name := range c.rawConfig.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SwitchContext changes to a different kubeconfig context and rebuilds the clientset.
func (c *Client) SwitchContext(ctx string) error {
	newClient, err := NewClient(c.kubeconfig, ctx)
	if err != nil {
		return fmt.Errorf("switching context to %q: %w", ctx, err)
	}
	*c = *newClient
	return nil
}

// Namespaces returns all namespace names.
func (c *Client) Namespaces(ctx context.Context) ([]string, error) {
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}
	names := make([]string, len(nsList.Items))
	for i, ns := range nsList.Items {
		names[i] = ns.Name
	}
	sort.Strings(names)
	return names, nil
}

// ListPods returns pods in the given namespace (or all if empty), filtered by phase.
// Metrics come from metrics-server (CPU/mem) and kubelet stats/summary (disk/network + CPU/mem fallback).
func (c *Client) ListPods(ctx context.Context, namespace string, filter PodPhaseFilter) ([]PodInfo, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	usageByPod, _ := c.fetchPodUsage(ctx, namespace)

	nodeNames := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != "" {
			nodeNames = append(nodeNames, pod.Spec.NodeName)
		}
	}
	_, kubeletPods := c.fetchKubeletEnrichment(ctx, nodeNames)

	var pods []PodInfo
	for _, pod := range podList.Items {
		if !matchesFilter(pod.Status.Phase, filter) {
			continue
		}
		info := toPodInfo(pod)
		metricsAPI := usageByPod[usageKey{namespace: pod.Namespace, name: pod.Name}]
		kubelet := kubeletPods[usageKey{namespace: pod.Namespace, name: pod.Name}]
		m := mergePodMetrics(pod, metricsAPI, kubelet)
		if m.CPULabel != "" || m.MemoryLabel != "" || m.DiskLabel != "" || m.NetworkLabel != "" {
			info.ApplyMetrics(m)
		}
		pods = append(pods, info)
	}

	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})

	return pods, nil
}

// GetPodLogs returns the logs for a specific pod and container.
// If container is empty, it uses the first container.
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, container string, tailLines int64, follow bool) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{
		Follow: follow,
	}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}
	if container != "" {
		opts.Container = container
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	return req.Stream(ctx)
}

// GetPodEnvVars returns the environment variables for all containers in a pod.
// It resolves three sources:
//  1. spec.containers[].envFrom — bulk injection from ConfigMaps and Secrets
//  2. spec.containers[].env — individual env vars (inline or referenced)
//  3. spec.initContainers[] — same treatment for init containers
//
// For envFrom entries, it fetches the referenced ConfigMap or Secret and
// expands every key into an individual EnvVar so the user sees the actual
// variable names. If the referenced resource cannot be read (RBAC, deleted),
// the entry falls back to a descriptive placeholder.
func (c *Client) GetPodEnvVars(ctx context.Context, namespace, podName string) ([]ContainerInfo, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, podName, err)
	}

	// Process regular containers then init containers.
	allContainers := make([]corev1.Container, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	allContainers = append(allContainers, pod.Spec.InitContainers...)
	allContainers = append(allContainers, pod.Spec.Containers...)

	var result []ContainerInfo
	for idx, ctr := range allContainers {
		ci := ContainerInfo{
			Name:  ctr.Name,
			Image: ctr.Image,
		}
		// Mark init containers so the UI can distinguish them.
		if idx < len(pod.Spec.InitContainers) {
			ci.Name = ctr.Name + " (init)"
		}

		// --- 1. envFrom: bulk-injected from ConfigMaps / Secrets ---
		for _, ef := range ctr.EnvFrom {
			prefix := ef.Prefix
			if ef.ConfigMapRef != nil {
				ci.EnvVars = append(ci.EnvVars,
					c.resolveConfigMapEnvFrom(ctx, namespace, ef.ConfigMapRef.Name, prefix)...)
			}
			if ef.SecretRef != nil {
				ci.EnvVars = append(ci.EnvVars,
					c.resolveSecretEnvFrom(ctx, namespace, ef.SecretRef.Name, prefix)...)
			}
		}

		// --- 2. env: individual vars (may override envFrom keys) ---
		for _, env := range ctr.Env {
			ev := EnvVar{Name: env.Name}
			if env.Value != "" {
				ev.Value = env.Value
			} else if env.ValueFrom != nil {
				ev.Value, ev.ValueFrom = c.resolveEnvVarValue(ctx, namespace, env.ValueFrom)
			}
			ci.EnvVars = append(ci.EnvVars, ev)
		}

		result = append(result, ci)
	}
	return result, nil
}

// resolveConfigMapEnvFrom fetches a ConfigMap and returns one EnvVar per key.
func (c *Client) resolveConfigMapEnvFrom(ctx context.Context, namespace, name, prefix string) []EnvVar {
	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return []EnvVar{{
			Name:      prefix + "*",
			ValueFrom: fmt.Sprintf("configmap:%s (unreadable: %v)", name, err),
		}}
	}
	vars := make([]EnvVar, 0, len(cm.Data))
	for k, v := range cm.Data {
		vars = append(vars, EnvVar{Name: prefix + k, Value: v})
	}
	return vars
}

// resolveSecretEnvFrom fetches a Secret and returns one EnvVar per key.
// Values are resolved to their actual content for operational use.
func (c *Client) resolveSecretEnvFrom(ctx context.Context, namespace, name, prefix string) []EnvVar {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return []EnvVar{{
			Name:      prefix + "*",
			ValueFrom: fmt.Sprintf("secret:%s (unreadable: %v)", name, err),
		}}
	}
	vars := make([]EnvVar, 0, len(secret.Data))
	for k, v := range secret.Data {
		vars = append(vars, EnvVar{
			Name:      prefix + k,
			ValueFrom: fmt.Sprintf("secret:%s/%s", name, k),
			Value:     string(v),
		})
	}
	return vars
}

// resolveEnvVarValue attempts to resolve a ValueFrom reference to its actual
// value. Returns (resolvedValue, sourceDescription).
func (c *Client) resolveEnvVarValue(ctx context.Context, namespace string, src *corev1.EnvVarSource) (string, string) {
	desc := describeValueSource(src)

	switch {
	case src.ConfigMapKeyRef != nil:
		ref := src.ConfigMapKeyRef
		cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err == nil {
			if val, ok := cm.Data[ref.Key]; ok {
				return val, desc
			}
		}
		return "", desc

	case src.SecretKeyRef != nil:
		ref := src.SecretKeyRef
		secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err == nil {
			if val, ok := secret.Data[ref.Key]; ok {
				return string(val), desc
			}
		}
		return "", desc

	case src.FieldRef != nil:
		// Downward API fields (metadata.name, status.podIP, etc.)
		// These are only resolved at runtime; show the field path.
		return "", desc

	case src.ResourceFieldRef != nil:
		return "", desc

	default:
		return "", desc
	}
}

// ListDeployments returns deployment info for the given namespace.
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	depList, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}

	var resources []ResourceInfo
	for _, d := range depList.Items {
		resources = append(resources, ResourceInfo{
			Kind:      "Deployment",
			Name:      d.Name,
			Namespace: d.Namespace,
			Age:       time.Since(d.CreationTimestamp.Time),
			Status:    fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, *d.Spec.Replicas),
			Extra: map[string]string{
				"images": extractDeploymentImages(d.Spec.Template.Spec.Containers),
			},
		})
	}
	return resources, nil
}

// ListServices returns service info for the given namespace.
func (c *Client) ListServices(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	svcList, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	var resources []ResourceInfo
	for _, s := range svcList.Items {
		ports := make([]string, len(s.Spec.Ports))
		for i, p := range s.Spec.Ports {
			ports[i] = fmt.Sprintf("%d/%s", p.Port, p.Protocol)
		}
		resources = append(resources, ResourceInfo{
			Kind:      "Service",
			Name:      s.Name,
			Namespace: s.Namespace,
			Age:       time.Since(s.CreationTimestamp.Time),
			Status:    string(s.Spec.Type),
			Extra: map[string]string{
				"clusterIP": s.Spec.ClusterIP,
				"ports":     strings.Join(ports, ", "),
			},
		})
	}
	return resources, nil
}

// ListEvents returns recent events for a namespace.
func (c *Client) ListEvents(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	// Sort by last timestamp descending.
	sort.Slice(eventList.Items, func(i, j int) bool {
		return eventList.Items[i].LastTimestamp.After(eventList.Items[j].LastTimestamp.Time)
	})

	// Return last 50 events.
	limit := min(50, len(eventList.Items))
	resources := make([]ResourceInfo, limit)
	for i := 0; i < limit; i++ {
		e := eventList.Items[i]
		resources[i] = ResourceInfo{
			Kind:      "Event",
			Name:      e.InvolvedObject.Name,
			Namespace: e.Namespace,
			Age:       time.Since(e.LastTimestamp.Time),
			Status:    e.Type,
			Extra: map[string]string{
				"reason":  e.Reason,
				"message": e.Message,
				"kind":    e.InvolvedObject.Kind,
				"count":   fmt.Sprintf("%d", e.Count),
			},
		}
	}
	return resources, nil
}

// ListNodes returns node info.
// Metrics come from metrics-server (CPU/mem) and kubelet stats/summary (disk/network + CPU/mem fallback).
func (c *Client) ListNodes(ctx context.Context) ([]ResourceInfo, error) {
	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	usageByNode, _ := c.fetchNodeUsage(ctx)
	nodeNames := make([]string, len(nodeList.Items))
	for i, n := range nodeList.Items {
		nodeNames[i] = n.Name
	}
	kubeletNodes, _ := c.fetchKubeletEnrichment(ctx, nodeNames)

	var resources []ResourceInfo
	for _, n := range nodeList.Items {
		status := "NotReady"
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				status = "Ready"
				break
			}
		}
		info := ResourceInfo{
			Kind:      "Node",
			Name:      n.Name,
			Namespace: "",
			Age:       time.Since(n.CreationTimestamp.Time),
			Status:    status,
			Extra: map[string]string{
				"version":          n.Status.NodeInfo.KubeletVersion,
				"os":               n.Status.NodeInfo.OSImage,
				"arch":             n.Status.NodeInfo.Architecture,
				"containerRuntime": n.Status.NodeInfo.ContainerRuntimeVersion,
			},
		}
		m := mergeNodeMetrics(n, usageByNode[n.Name], kubeletNodes[n.Name])
		if m.CPULabel != "" || m.MemoryLabel != "" || m.DiskLabel != "" || m.NetworkLabel != "" {
			info.ApplyMetrics(m)
		}
		resources = append(resources, info)
	}
	return resources, nil
}

// ListConfigMaps returns configmap names in a namespace.
func (c *Client) ListConfigMaps(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	cmList, err := c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing configmaps: %w", err)
	}

	resources := make([]ResourceInfo, len(cmList.Items))
	for i, cm := range cmList.Items {
		resources[i] = ResourceInfo{
			Kind:      "ConfigMap",
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Age:       time.Since(cm.CreationTimestamp.Time),
			Extra: map[string]string{
				"keys": fmt.Sprintf("%d", len(cm.Data)),
			},
		}
	}
	return resources, nil
}

// ListSecrets returns secret names (not data) in a namespace.
func (c *Client) ListSecrets(ctx context.Context, namespace string) ([]ResourceInfo, error) {
	secretList, err := c.clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	resources := make([]ResourceInfo, len(secretList.Items))
	for i, s := range secretList.Items {
		resources[i] = ResourceInfo{
			Kind:      "Secret",
			Name:      s.Name,
			Namespace: s.Namespace,
			Age:       time.Since(s.CreationTimestamp.Time),
			Status:    string(s.Type),
			Extra: map[string]string{
				"keys": fmt.Sprintf("%d", len(s.Data)),
			},
		}
	}
	return resources, nil
}

// GetPodsBySelector returns pods matching a label selector.
func (c *Client) GetPodsBySelector(ctx context.Context, namespace string, selector labels.Selector) ([]PodInfo, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("listing pods by selector: %w", err)
	}

	pods := make([]PodInfo, len(podList.Items))
	for i, pod := range podList.Items {
		pods[i] = toPodInfo(pod)
	}
	return pods, nil
}

// DescribePod returns detailed pod information.
func (c *Client) DescribePod(ctx context.Context, namespace, name string) (*PodInfo, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting pod %s/%s: %w", namespace, name, err)
	}
	info := toPodInfo(*pod)
	return &info, nil
}

// DeletePod deletes a pod.
func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	return c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ScaleDeployment scales a deployment to the given replica count.
func (c *Client) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	scale, err := c.clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting scale: %w", err)
	}
	scale.Spec.Replicas = replicas
	_, err = c.clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	return err
}

// RestartDeployment triggers a rolling restart by patching the deployment annotation.
func (c *Client) RestartDeployment(ctx context.Context, namespace, name string) error {
	dep, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting deployment: %w", err)
	}

	if dep.Spec.Template.ObjectMeta.Annotations == nil {
		dep.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	dep.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}

// ServerVersion returns the Kubernetes server version string.
func (c *Client) ServerVersion() (string, error) {
	info, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

// --- Helper functions ---

func matchesFilter(phase corev1.PodPhase, filter PodPhaseFilter) bool {
	switch filter {
	case PodFilterAll:
		return true
	case PodFilterRunning:
		return phase == corev1.PodRunning
	case PodFilterNotRunning:
		return phase != corev1.PodRunning
	case PodFilterFailed:
		return phase == corev1.PodFailed
	case PodFilterPending:
		return phase == corev1.PodPending
	case PodFilterSucceeded:
		return phase == corev1.PodSucceeded
	}
	return true
}

func toPodInfo(pod corev1.Pod) PodInfo {
	var restarts int32
	readyCount := 0
	totalCount := len(pod.Spec.Containers)
	var containers []ContainerInfo

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyCount++
		}
		restarts += cs.RestartCount

		state := "Unknown"
		if cs.State.Running != nil {
			state = "Running"
		} else if cs.State.Waiting != nil {
			state = "Waiting: " + cs.State.Waiting.Reason
		} else if cs.State.Terminated != nil {
			state = "Terminated: " + cs.State.Terminated.Reason
		}

		containers = append(containers, ContainerInfo{
			Name:  cs.Name,
			Image: cs.Image,
			Ready: cs.Ready,
			State: state,
		})
	}

	images := make([]string, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		images[i] = c.Image
	}

	return PodInfo{
		Name:       pod.Name,
		Namespace:  pod.Namespace,
		Phase:      pod.Status.Phase,
		Ready:      fmt.Sprintf("%d/%d", readyCount, totalCount),
		Restarts:   restarts,
		Age:        time.Since(pod.CreationTimestamp.Time),
		Node:       pod.Spec.NodeName,
		Images:     images,
		Labels:     pod.Labels,
		Containers: containers,
	}
}

func describeValueSource(src *corev1.EnvVarSource) string {
	switch {
	case src.ConfigMapKeyRef != nil:
		return fmt.Sprintf("configmap:%s/%s", src.ConfigMapKeyRef.Name, src.ConfigMapKeyRef.Key)
	case src.SecretKeyRef != nil:
		return fmt.Sprintf("secret:%s/%s", src.SecretKeyRef.Name, src.SecretKeyRef.Key)
	case src.FieldRef != nil:
		return fmt.Sprintf("field:%s", src.FieldRef.FieldPath)
	case src.ResourceFieldRef != nil:
		return fmt.Sprintf("resource:%s/%s", src.ResourceFieldRef.ContainerName, src.ResourceFieldRef.Resource)
	default:
		return "unknown"
	}
}

func extractDeploymentImages(containers []corev1.Container) string {
	images := make([]string, len(containers))
	for i, c := range containers {
		images[i] = c.Image
	}
	return strings.Join(images, ", ")
}
