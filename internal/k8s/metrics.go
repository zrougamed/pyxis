package k8s

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// SparklineSample is one CPU/memory utilisation point for sparkline history.
type SparklineSample struct {
	CPUPercent float64
	MemPercent float64
	At         time.Time
}

// MetricsHistory is a tiny in-memory ring buffer of utilisation samples.
type MetricsHistory struct {
	mu      sync.Mutex
	cap     int
	samples []SparklineSample
	next    int
	full    bool
}

// NewMetricsHistory creates a ring buffer that retains up to capacity samples.
func NewMetricsHistory(capacity int) *MetricsHistory {
	if capacity < 1 {
		capacity = 60
	}
	return &MetricsHistory{
		cap:     capacity,
		samples: make([]SparklineSample, capacity),
	}
}

// Add appends a sample, overwriting the oldest when full.
func (h *MetricsHistory) Add(s SparklineSample) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if s.At.IsZero() {
		s.At = time.Now()
	}
	h.samples[h.next] = s
	h.next = (h.next + 1) % h.cap
	if h.next == 0 {
		h.full = true
	}
}

// Len returns the number of stored samples.
func (h *MetricsHistory) Len() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.full {
		return h.cap
	}
	return h.next
}

// Samples returns samples in chronological order (oldest first).
func (h *MetricsHistory) Samples() []SparklineSample {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.full {
		out := make([]SparklineSample, h.next)
		copy(out, h.samples[:h.next])
		return out
	}
	out := make([]SparklineSample, h.cap)
	copy(out, h.samples[h.next:])
	copy(out[h.cap-h.next:], h.samples[:h.next])
	return out
}

// CPUPercents returns CPU percent values in chronological order.
func (h *MetricsHistory) CPUPercents() []float64 {
	samples := h.Samples()
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s.CPUPercent
	}
	return out
}

// MemPercents returns memory percent values in chronological order.
func (h *MetricsHistory) MemPercents() []float64 {
	samples := h.Samples()
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s.MemPercent
	}
	return out
}

// ResourceMetrics holds usage values suitable for gauges and API responses.
type ResourceMetrics struct {
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

type usageKey struct {
	namespace string
	name      string
}

type rawUsage struct {
	cpuMilli int64
	memBytes int64
}

// MetricsAvailable reports whether a metrics-server client was configured.
func (c *Client) MetricsAvailable() bool {
	return c != nil && c.metricsClient != nil
}

func (c *Client) fetchPodUsage(ctx context.Context, namespace string) (map[usageKey]rawUsage, error) {
	if c.metricsClient == nil {
		return nil, nil
	}
	list, err := c.metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make(map[usageKey]rawUsage, len(list.Items))
	for _, item := range list.Items {
		out[usageKey{namespace: item.Namespace, name: item.Name}] = sumContainerUsage(item.Containers)
	}
	return out, nil
}

func (c *Client) fetchNodeUsage(ctx context.Context) (map[string]rawUsage, error) {
	if c.metricsClient == nil {
		return nil, nil
	}
	list, err := c.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make(map[string]rawUsage, len(list.Items))
	for _, item := range list.Items {
		out[item.Name] = rawUsage{
			cpuMilli: quantityMilliCPU(item.Usage[corev1.ResourceCPU]),
			memBytes: quantityBytes(item.Usage[corev1.ResourceMemory]),
		}
	}
	return out, nil
}

func sumContainerUsage(containers []metricsv1beta1.ContainerMetrics) rawUsage {
	var u rawUsage
	for _, c := range containers {
		u.cpuMilli += quantityMilliCPU(c.Usage[corev1.ResourceCPU])
		u.memBytes += quantityBytes(c.Usage[corev1.ResourceMemory])
	}
	return u
}

func podMetricsFromUsage(pod corev1.Pod, usage rawUsage) ResourceMetrics {
	m := ResourceMetrics{
		CPUMillicores: usage.cpuMilli,
		MemoryBytes:   usage.memBytes,
		CPULabel:      FormatCPUMillicores(usage.cpuMilli),
		MemoryLabel:   FormatMemoryBytes(usage.memBytes),
	}

	reqCPU, reqMem := podResourceRequests(pod)
	if reqCPU > 0 {
		m.CPUPercent = percentPtr(usage.cpuMilli, reqCPU)
	}
	if reqMem > 0 {
		m.MemoryPercent = percentPtr(usage.memBytes, reqMem)
	}
	return m
}

func nodeMetricsFromUsage(node corev1.Node, usage rawUsage) ResourceMetrics {
	m := ResourceMetrics{
		CPUMillicores: usage.cpuMilli,
		MemoryBytes:   usage.memBytes,
		CPULabel:      FormatCPUMillicores(usage.cpuMilli),
		MemoryLabel:   FormatMemoryBytes(usage.memBytes),
	}

	allocCPU := quantityMilliCPU(node.Status.Allocatable[corev1.ResourceCPU])
	allocMem := quantityBytes(node.Status.Allocatable[corev1.ResourceMemory])
	if allocCPU > 0 {
		m.CPUPercent = percentPtr(usage.cpuMilli, allocCPU)
	}
	if allocMem > 0 {
		m.MemoryPercent = percentPtr(usage.memBytes, allocMem)
	}
	return m
}

func podResourceRequests(pod corev1.Pod) (cpuMilli, memBytes int64) {
	for _, c := range pod.Spec.Containers {
		cpuMilli += quantityMilliCPU(c.Resources.Requests[corev1.ResourceCPU])
		memBytes += quantityBytes(c.Resources.Requests[corev1.ResourceMemory])
	}
	return cpuMilli, memBytes
}

func quantityMilliCPU(q resource.Quantity) int64 {
	if q.IsZero() {
		return 0
	}
	return q.MilliValue()
}

func quantityBytes(q resource.Quantity) int64 {
	if q.IsZero() {
		return 0
	}
	return q.Value()
}

func percentPtr(used, capacity int64) *float64 {
	if capacity <= 0 {
		return nil
	}
	pct := (float64(used) / float64(capacity)) * 100
	if pct < 0 {
		pct = 0
	}
	// Cap display at 999 so gauges stay readable when over-request.
	if pct > 999 {
		pct = 999
	}
	pct = math.Round(pct*10) / 10
	return &pct
}

// FormatCPUMillicores renders millicores as "125m" or "1.5".
func FormatCPUMillicores(milli int64) string {
	if milli < 1000 {
		return fmt.Sprintf("%dm", milli)
	}
	cores := float64(milli) / 1000
	if cores == float64(int64(cores)) {
		return fmt.Sprintf("%d", int64(cores))
	}
	return fmt.Sprintf("%.1f", cores)
}

// FormatMemoryBytes renders bytes as Ki/Mi/Gi.
func FormatMemoryBytes(bytes int64) string {
	const (
		ki = 1024
		mi = 1024 * ki
		gi = 1024 * mi
	)
	switch {
	case bytes >= gi:
		v := float64(bytes) / float64(gi)
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dGi", int64(v))
		}
		return fmt.Sprintf("%.1fGi", v)
	case bytes >= mi:
		v := float64(bytes) / float64(mi)
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dMi", int64(v))
		}
		return fmt.Sprintf("%.1fMi", v)
	case bytes >= ki:
		return fmt.Sprintf("%dKi", bytes/ki)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// ApplyPodMetrics copies metric fields onto PodInfo.
func (p *PodInfo) ApplyMetrics(m ResourceMetrics) {
	p.CPUMillicores = m.CPUMillicores
	p.MemoryBytes = m.MemoryBytes
	p.DiskBytes = m.DiskBytes
	p.NetworkRxBytes = m.NetworkRxBytes
	p.NetworkTxBytes = m.NetworkTxBytes
	p.CPULabel = m.CPULabel
	p.MemoryLabel = m.MemoryLabel
	p.DiskLabel = m.DiskLabel
	p.NetworkLabel = m.NetworkLabel
	p.CPUPercent = m.CPUPercent
	p.MemoryPercent = m.MemoryPercent
	p.DiskPercent = m.DiskPercent
	p.NetworkPercent = m.NetworkPercent
}

// ApplyMetrics copies metric fields onto ResourceInfo (nodes/workloads).
func (r *ResourceInfo) ApplyMetrics(m ResourceMetrics) {
	r.CPUMillicores = m.CPUMillicores
	r.MemoryBytes = m.MemoryBytes
	r.DiskBytes = m.DiskBytes
	r.NetworkRxBytes = m.NetworkRxBytes
	r.NetworkTxBytes = m.NetworkTxBytes
	r.CPULabel = m.CPULabel
	r.MemoryLabel = m.MemoryLabel
	r.DiskLabel = m.DiskLabel
	r.NetworkLabel = m.NetworkLabel
	r.CPUPercent = m.CPUPercent
	r.MemoryPercent = m.MemoryPercent
	r.DiskPercent = m.DiskPercent
	r.NetworkPercent = m.NetworkPercent
}
