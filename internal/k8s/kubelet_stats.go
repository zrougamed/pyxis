package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// kubeletSummary is the subset of /stats/summary we need for OpenLens-like gauges.
type kubeletSummary struct {
	Node kubeletNodeStats  `json:"node"`
	Pods []kubeletPodStats `json:"pods"`
}

type kubeletNodeStats struct {
	NodeName string           `json:"nodeName"`
	CPU      *kubeletCPU      `json:"cpu"`
	Memory   *kubeletMemory   `json:"memory"`
	Network  *kubeletNetwork  `json:"network"`
	Fs       *kubeletFsStats  `json:"fs"`
	Runtime  *kubeletRuntime  `json:"runtime"`
}

type kubeletRuntime struct {
	ImageFs *kubeletFsStats `json:"imageFs"`
}

type kubeletPodStats struct {
	PodRef            kubeletPodRef   `json:"podRef"`
	CPU               *kubeletCPU     `json:"cpu"`
	Memory            *kubeletMemory  `json:"memory"`
	Network           *kubeletNetwork `json:"network"`
	EphemeralStorage  *kubeletFsStats `json:"ephemeral-storage"`
	Containers        []kubeletContainerStats `json:"containers"`
}

type kubeletPodRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type kubeletContainerStats struct {
	Name   string          `json:"name"`
	CPU    *kubeletCPU     `json:"cpu"`
	Memory *kubeletMemory  `json:"memory"`
	Rootfs *kubeletFsStats `json:"rootfs"`
	Logs   *kubeletFsStats `json:"logs"`
}

type kubeletCPU struct {
	UsageNanoCores int64 `json:"usageNanoCores"`
}

type kubeletMemory struct {
	WorkingSetBytes uint64 `json:"workingSetBytes"`
	UsageBytes      uint64 `json:"usageBytes"`
}

type kubeletNetwork struct {
	RxBytes  uint64 `json:"rxBytes"`
	TxBytes  uint64 `json:"txBytes"`
	RxErrors uint64 `json:"rxErrors"`
	TxErrors uint64 `json:"txErrors"`
}

type kubeletFsStats struct {
	AvailableBytes *uint64 `json:"availableBytes"`
	CapacityBytes  *uint64 `json:"capacityBytes"`
	UsedBytes      *uint64 `json:"usedBytes"`
}

type enrichedUsage struct {
	cpuMilli   int64
	memBytes   int64
	diskUsed   int64
	diskCap    int64
	netRx      int64
	netTx      int64
	hasCPU     bool
	hasMem     bool
	hasDisk    bool
	hasNetwork bool
}

func (c *Client) fetchNodeSummary(ctx context.Context, nodeName string) (*kubeletSummary, error) {
	if c == nil || c.clientset == nil || c.config == nil || nodeName == "" {
		return nil, fmt.Errorf("kubelet summary unavailable")
	}
	restClient := c.clientset.CoreV1().RESTClient()
	if restClient == nil {
		return nil, fmt.Errorf("kubelet summary unavailable")
	}
	req := restClient.Get().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy").
		Suffix("stats/summary")

	raw, err := req.DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	var summary kubeletSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *Client) fetchKubeletEnrichment(ctx context.Context, nodeNames []string) (nodes map[string]enrichedUsage, pods map[usageKey]enrichedUsage) {
	nodes = map[string]enrichedUsage{}
	pods = map[usageKey]enrichedUsage{}
	seen := map[string]struct{}{}
	for _, name := range nodeNames {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		summary, err := c.fetchNodeSummary(ctx, name)
		if err != nil {
			continue
		}
		nodes[name] = nodeUsageFromSummary(summary)
		for _, pod := range summary.Pods {
			key := usageKey{namespace: pod.PodRef.Namespace, name: pod.PodRef.Name}
			pods[key] = podUsageFromSummary(pod)
		}
	}
	return nodes, pods
}

func nodeUsageFromSummary(summary *kubeletSummary) enrichedUsage {
	var u enrichedUsage
	if summary == nil {
		return u
	}
	n := summary.Node
	if n.CPU != nil && n.CPU.UsageNanoCores > 0 {
		u.cpuMilli = n.CPU.UsageNanoCores / 1_000_000
		u.hasCPU = true
	}
	if n.Memory != nil {
		bytes := n.Memory.WorkingSetBytes
		if bytes == 0 {
			bytes = n.Memory.UsageBytes
		}
		if bytes > 0 {
			u.memBytes = int64(bytes)
			u.hasMem = true
		}
	}
	fs := n.Fs
	if fs == nil && n.Runtime != nil {
		fs = n.Runtime.ImageFs
	}
	if fs != nil && fs.UsedBytes != nil {
		u.diskUsed = int64(*fs.UsedBytes)
		u.hasDisk = true
		if fs.CapacityBytes != nil {
			u.diskCap = int64(*fs.CapacityBytes)
		}
	}
	if n.Network != nil {
		u.netRx = int64(n.Network.RxBytes)
		u.netTx = int64(n.Network.TxBytes)
		u.hasNetwork = u.netRx > 0 || u.netTx > 0
	}
	return u
}

func podUsageFromSummary(pod kubeletPodStats) enrichedUsage {
	var u enrichedUsage
	if pod.CPU != nil && pod.CPU.UsageNanoCores > 0 {
		u.cpuMilli = pod.CPU.UsageNanoCores / 1_000_000
		u.hasCPU = true
	}
	if pod.Memory != nil {
		bytes := pod.Memory.WorkingSetBytes
		if bytes == 0 {
			bytes = pod.Memory.UsageBytes
		}
		if bytes > 0 {
			u.memBytes = int64(bytes)
			u.hasMem = true
		}
	}
	if !u.hasCPU || !u.hasMem || !u.hasDisk {
		var cpuSum int64
		var memSum int64
		var diskSum int64
		for _, ctr := range pod.Containers {
			if ctr.CPU != nil {
				cpuSum += ctr.CPU.UsageNanoCores / 1_000_000
			}
			if ctr.Memory != nil {
				bytes := ctr.Memory.WorkingSetBytes
				if bytes == 0 {
					bytes = ctr.Memory.UsageBytes
				}
				memSum += int64(bytes)
			}
			if ctr.Rootfs != nil && ctr.Rootfs.UsedBytes != nil {
				diskSum += int64(*ctr.Rootfs.UsedBytes)
			}
			if ctr.Logs != nil && ctr.Logs.UsedBytes != nil {
				diskSum += int64(*ctr.Logs.UsedBytes)
			}
		}
		if !u.hasCPU && cpuSum > 0 {
			u.cpuMilli = cpuSum
			u.hasCPU = true
		}
		if !u.hasMem && memSum > 0 {
			u.memBytes = memSum
			u.hasMem = true
		}
		if diskSum > 0 {
			u.diskUsed = diskSum
			u.hasDisk = true
		}
	}
	if pod.EphemeralStorage != nil {
		if pod.EphemeralStorage.UsedBytes != nil {
			u.diskUsed = int64(*pod.EphemeralStorage.UsedBytes)
			u.hasDisk = true
		}
		if pod.EphemeralStorage.CapacityBytes != nil {
			u.diskCap = int64(*pod.EphemeralStorage.CapacityBytes)
		}
	}
	if pod.Network != nil {
		u.netRx = int64(pod.Network.RxBytes)
		u.netTx = int64(pod.Network.TxBytes)
		u.hasNetwork = u.netRx > 0 || u.netTx > 0
	}
	return u
}

func mergePodMetrics(pod corev1.Pod, metricsAPI rawUsage, kubelet enrichedUsage) ResourceMetrics {
	cpu := metricsAPI.cpuMilli
	mem := metricsAPI.memBytes
	if cpu == 0 && kubelet.hasCPU {
		cpu = kubelet.cpuMilli
	}
	if mem == 0 && kubelet.hasMem {
		mem = kubelet.memBytes
	}

	m := ResourceMetrics{}
	if cpu > 0 {
		m.CPUMillicores = cpu
		m.CPULabel = FormatCPUMillicores(cpu)
	}
	if mem > 0 {
		m.MemoryBytes = mem
		m.MemoryLabel = FormatMemoryBytes(mem)
	}

	reqCPU, reqMem := podResourceRequests(pod)
	limCPU, limMem := podResourceLimits(pod)
	if reqCPU > 0 {
		m.CPUPercent = percentPtr(cpu, reqCPU)
	} else if limCPU > 0 && cpu > 0 {
		m.CPUPercent = percentPtr(cpu, limCPU)
	}
	if reqMem > 0 {
		m.MemoryPercent = percentPtr(mem, reqMem)
	} else if limMem > 0 && mem > 0 {
		m.MemoryPercent = percentPtr(mem, limMem)
	}

	if kubelet.hasDisk {
		m.DiskBytes = kubelet.diskUsed
		m.DiskLabel = FormatMemoryBytes(kubelet.diskUsed)
		if kubelet.diskCap > 0 {
			m.DiskPercent = percentPtr(kubelet.diskUsed, kubelet.diskCap)
			m.DiskLabel = fmt.Sprintf("%s / %s", FormatMemoryBytes(kubelet.diskUsed), FormatMemoryBytes(kubelet.diskCap))
		}
	}
	if kubelet.hasNetwork {
		m.NetworkRxBytes = kubelet.netRx
		m.NetworkTxBytes = kubelet.netTx
		m.NetworkLabel = formatNetworkLabel(kubelet.netRx, kubelet.netTx)
		// No hard capacity for cumulative counters; leave percent unset.
	}
	return m
}

func mergeNodeMetrics(node corev1.Node, metricsAPI rawUsage, kubelet enrichedUsage) ResourceMetrics {
	cpu := metricsAPI.cpuMilli
	mem := metricsAPI.memBytes
	if cpu == 0 && kubelet.hasCPU {
		cpu = kubelet.cpuMilli
	}
	if mem == 0 && kubelet.hasMem {
		mem = kubelet.memBytes
	}

	m := ResourceMetrics{}
	if cpu > 0 {
		m.CPUMillicores = cpu
		m.CPULabel = FormatCPUMillicores(cpu)
	}
	if mem > 0 {
		m.MemoryBytes = mem
		m.MemoryLabel = FormatMemoryBytes(mem)
	}

	allocCPU := quantityMilliCPU(node.Status.Allocatable[corev1.ResourceCPU])
	allocMem := quantityBytes(node.Status.Allocatable[corev1.ResourceMemory])
	if allocCPU > 0 && cpu > 0 {
		m.CPUPercent = percentPtr(cpu, allocCPU)
	}
	if allocMem > 0 && mem > 0 {
		m.MemoryPercent = percentPtr(mem, allocMem)
	}

	if kubelet.hasDisk {
		m.DiskBytes = kubelet.diskUsed
		m.DiskLabel = FormatMemoryBytes(kubelet.diskUsed)
		if kubelet.diskCap > 0 {
			m.DiskPercent = percentPtr(kubelet.diskUsed, kubelet.diskCap)
			m.DiskLabel = fmt.Sprintf("%s / %s", FormatMemoryBytes(kubelet.diskUsed), FormatMemoryBytes(kubelet.diskCap))
		}
	}
	if kubelet.hasNetwork {
		m.NetworkRxBytes = kubelet.netRx
		m.NetworkTxBytes = kubelet.netTx
		m.NetworkLabel = formatNetworkLabel(kubelet.netRx, kubelet.netTx)
	}
	return m
}

func formatNetworkLabel(rx, tx int64) string {
	return fmt.Sprintf("↓%s ↑%s", FormatMemoryBytes(rx), FormatMemoryBytes(tx))
}

func podResourceLimits(pod corev1.Pod) (cpuMilli, memBytes int64) {
	for _, c := range pod.Spec.Containers {
		cpuMilli += quantityMilliCPU(c.Resources.Limits[corev1.ResourceCPU])
		memBytes += quantityBytes(c.Resources.Limits[corev1.ResourceMemory])
	}
	return cpuMilli, memBytes
}
