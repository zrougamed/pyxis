package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatCPUMillicores(t *testing.T) {
	tests := []struct {
		milli int64
		want  string
	}{
		{0, "0m"},
		{125, "125m"},
		{1000, "1"},
		{1500, "1.5"},
	}
	for _, tt := range tests {
		if got := FormatCPUMillicores(tt.milli); got != tt.want {
			t.Errorf("FormatCPUMillicores(%d) = %q, want %q", tt.milli, got, tt.want)
		}
	}
}

func TestFormatMemoryBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{512, "512B"},
		{2048, "2Ki"},
		{256 * 1024 * 1024, "256Mi"},
		{2 * 1024 * 1024 * 1024, "2Gi"},
	}
	for _, tt := range tests {
		if got := FormatMemoryBytes(tt.bytes); got != tt.want {
			t.Errorf("FormatMemoryBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestPercentPtr(t *testing.T) {
	if percentPtr(50, 0) != nil {
		t.Fatal("expected nil when capacity is 0")
	}
	got := percentPtr(50, 100)
	if got == nil || *got != 50 {
		t.Fatalf("expected 50, got %v", got)
	}
}

func TestPodMetricsFromUsageUsesRequests(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "main",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			}},
		},
	}
	m := podMetricsFromUsage(pod, rawUsage{cpuMilli: 250, memBytes: 256 * 1024 * 1024})
	if m.CPULabel != "250m" {
		t.Fatalf("CPULabel = %q", m.CPULabel)
	}
	if m.CPUPercent == nil || *m.CPUPercent != 50 {
		t.Fatalf("CPUPercent = %v, want 50", m.CPUPercent)
	}
	if m.MemoryPercent == nil || *m.MemoryPercent != 50 {
		t.Fatalf("MemoryPercent = %v, want 50", m.MemoryPercent)
	}
}

func TestNodeMetricsFromUsageUsesAllocatable(t *testing.T) {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}
	m := nodeMetricsFromUsage(node, rawUsage{cpuMilli: 2000, memBytes: 2 * 1024 * 1024 * 1024})
	if m.CPUPercent == nil || *m.CPUPercent != 50 {
		t.Fatalf("CPUPercent = %v, want 50", m.CPUPercent)
	}
	if m.MemoryPercent == nil || *m.MemoryPercent != 25 {
		t.Fatalf("MemoryPercent = %v, want 25", m.MemoryPercent)
	}
}

func TestListPodsWithoutMetricsClient(t *testing.T) {
	c := newTestClient(makePod("nginx", "default", corev1.PodRunning, "nginx:latest"))
	pods, err := c.ListPods(t.Context(), "default", PodFilterAll)
	if err != nil {
		t.Fatalf("ListPods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	if pods[0].CPULabel != "" {
		t.Fatalf("expected empty metrics without metrics client, got %q", pods[0].CPULabel)
	}
}
