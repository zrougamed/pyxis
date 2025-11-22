package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zrougamed/pyxis/internal/clipboard"
	"github.com/zrougamed/pyxis/internal/k8s"
	"github.com/zrougamed/pyxis/internal/tui/components"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// LaunchOptions configures the initial TUI screen (CLI deep-links).
type LaunchOptions struct {
	InitialView View
	Search      string
	TailLines   int64
}

// NewModelWithOptions creates a model that opens directly into a view.
func NewModelWithOptions(client *k8s.Client, namespace string, opts LaunchOptions) Model {
	m := NewModel(client, namespace)
	if opts.InitialView != ViewMenu {
		m.view = opts.InitialView
		m.list.Title = m.viewTitle()
		m.pendingSearch = opts.Search
		if opts.TailLines > 0 {
			m.logTail = opts.TailLines
		}
	}
	return m
}

type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmDeletePod
	confirmDeleteNamespace
	confirmRestart
	confirmScale
	confirmApplyYAML
)

type portForwardSession struct {
	ID        string
	Namespace string
	Pod       string
	Local     int
	Remote    int
	Stop      func()
}

type overviewData struct {
	NodeCount, NodesReady                         int
	PodCount, RunningPods, SucceededPods          int
	PendingPods, FailedPods                       int
	NamespaceCount, WarningEvents                 int
	Deployments, StatefulSets, DaemonSets, Jobs   int
	Services, Ingresses, PVCs, HPAs               int
	CPUPercent, MemPercent, DiskPercent           *float64
}

type overviewMsg struct {
	data overviewData
	err  error
}

type nsExploreMsg struct {
	name        string
	collections []nsCollection
	err         error
}

type nsCollection struct {
	ID    string
	Title string
	Count int
	Kind  string
	View  View
}

type yamlMsg struct {
	kind, namespace, name, content string
	err                         error
}

type watchTickMsg struct{}

type shellDoneMsg struct{ err error }

var nsExploreCatalog = []struct {
	ID    string
	Title string
	Kind  string
	View  View
	Pods  bool
}{
	{ID: "pods", Title: "Pods", Kind: "Pod", View: ViewPodList, Pods: true},
	{ID: "deployments", Title: "Deployments", Kind: "Deployment", View: ViewDeployments},
	{ID: "statefulsets", Title: "StatefulSets", Kind: "StatefulSet", View: ViewStatefulSets},
	{ID: "daemonsets", Title: "DaemonSets", Kind: "DaemonSet", View: ViewDaemonSets},
	{ID: "jobs", Title: "Jobs", Kind: "Job", View: ViewJobs},
	{ID: "cronjobs", Title: "CronJobs", Kind: "CronJob", View: ViewCronJobs},
	{ID: "services", Title: "Services", Kind: "Service", View: ViewServices},
	{ID: "ingresses", Title: "Ingresses", Kind: "Ingress", View: ViewIngresses},
	{ID: "configmaps", Title: "ConfigMaps", Kind: "ConfigMap", View: ViewConfigMaps},
	{ID: "secrets", Title: "Secrets", Kind: "Secret", View: ViewSecrets},
	{ID: "pvcs", Title: "PVCs", Kind: "PersistentVolumeClaim", View: ViewPVCs},
	{ID: "hpas", Title: "HPAs", Kind: "HorizontalPodAutoscaler", View: ViewHPAs},
	{ID: "events", Title: "Events", Kind: "Event", View: ViewEvents},
}

func (m Model) Init() tea.Cmd {
	if m.view != ViewMenu {
		return m.loadViewData(m.view)
	}
	return nil
}

func (m Model) loadOverview() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var data overviewData

		nodes, err := m.client.ListNodes(ctx)
		if err != nil {
			return overviewMsg{err: err}
		}
		data.NodeCount = len(nodes)
		for _, n := range nodes {
			if n.Status == "Ready" {
				data.NodesReady++
			}
		}
		data.CPUPercent = avgPercent(nodes, func(r k8s.ResourceInfo) *float64 { return r.CPUPercent })
		data.MemPercent = avgPercent(nodes, func(r k8s.ResourceInfo) *float64 { return r.MemoryPercent })
		data.DiskPercent = avgPercent(nodes, func(r k8s.ResourceInfo) *float64 { return r.DiskPercent })

		pods, err := m.client.ListPods(ctx, "", k8s.PodFilterAll)
		if err != nil {
			return overviewMsg{err: err}
		}
		data.PodCount = len(pods)
		for _, p := range pods {
			switch p.Phase {
			case "Running":
				data.RunningPods++
			case "Succeeded":
				data.SucceededPods++
			case "Pending":
				data.PendingPods++
			case "Failed":
				data.FailedPods++
			}
		}

		ns, err := m.client.Namespaces(ctx)
		if err == nil {
			data.NamespaceCount = len(ns)
		}

		events, err := m.client.ListEvents(ctx, "")
		if err == nil {
			for _, e := range events {
				if e.Status == "Warning" {
					data.WarningEvents++
				}
			}
		}

		count := func(fn func(context.Context, string) ([]k8s.ResourceInfo, error)) int {
			items, err := fn(ctx, "")
			if err != nil {
				return 0
			}
			return len(items)
		}
		data.Deployments = count(m.client.ListDeployments)
		data.StatefulSets = count(m.client.ListStatefulSets)
		data.DaemonSets = count(m.client.ListDaemonSets)
		data.Jobs = count(m.client.ListJobs)
		data.Services = count(m.client.ListServices)
		data.Ingresses = count(m.client.ListIngresses)
		data.PVCs = count(m.client.ListPersistentVolumeClaims)
		data.HPAs = count(m.client.ListHPAs)

		return overviewMsg{data: data}
	}
}

func avgPercent(items []k8s.ResourceInfo, get func(k8s.ResourceInfo) *float64) *float64 {
	var sum float64
	var n int
	for _, item := range items {
		if p := get(item); p != nil {
			sum += *p
			n++
		}
	}
	if n == 0 {
		return nil
	}
	avg := sum / float64(n)
	return &avg
}

func (m Model) loadNamespaceExplore(name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		collections := make([]nsCollection, 0, len(nsExploreCatalog))
		for _, entry := range nsExploreCatalog {
			col := nsCollection{ID: entry.ID, Title: entry.Title, Kind: entry.Kind, View: entry.View}
			if entry.Pods {
				pods, err := m.client.ListPods(ctx, name, k8s.PodFilterAll)
				if err == nil {
					col.Count = len(pods)
				}
			} else {
				var (
					items []k8s.ResourceInfo
					err   error
				)
				switch entry.View {
				case ViewDeployments:
					items, err = m.client.ListDeployments(ctx, name)
				case ViewStatefulSets:
					items, err = m.client.ListStatefulSets(ctx, name)
				case ViewDaemonSets:
					items, err = m.client.ListDaemonSets(ctx, name)
				case ViewJobs:
					items, err = m.client.ListJobs(ctx, name)
				case ViewCronJobs:
					items, err = m.client.ListCronJobs(ctx, name)
				case ViewServices:
					items, err = m.client.ListServices(ctx, name)
				case ViewIngresses:
					items, err = m.client.ListIngresses(ctx, name)
				case ViewConfigMaps:
					items, err = m.client.ListConfigMaps(ctx, name)
				case ViewSecrets:
					items, err = m.client.ListSecrets(ctx, name)
				case ViewPVCs:
					items, err = m.client.ListPersistentVolumeClaims(ctx, name)
				case ViewHPAs:
					items, err = m.client.ListHPAs(ctx, name)
				case ViewEvents:
					items, err = m.client.ListEvents(ctx, name)
				}
				if err == nil {
					col.Count = len(items)
				}
			}
			collections = append(collections, col)
		}
		return nsExploreMsg{name: name, collections: collections}
	}
}

func (m Model) fetchYAML(kind, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.GetResourceYAML(context.Background(), kind, namespace, name)
		return yamlMsg{kind: kind, namespace: namespace, name: name, content: content, err: err}
	}
}

func (m Model) kindForCurrentView() (kind, ns, name string, ok bool) {
	switch m.view {
	case ViewPodList, ViewPodDetail, ViewPodLogs, ViewPodEnv:
		pod, found := m.selectedPod()
		if !found && m.view == ViewPodDetail && m.selectedItem != "" {
			return "Pod", m.selectedNS, m.selectedItem, true
		}
		if !found {
			return "", "", "", false
		}
		return "Pod", pod.Namespace, pod.Name, true
	case ViewYAMLDetail:
		if m.yamlKind != "" && m.yamlName != "" {
			return m.yamlKind, m.yamlNS, m.yamlName, true
		}
	}
	res, found := m.selectedResource()
	if !found {
		return "", "", "", false
	}
	kind = res.Kind
	if kind == "" {
		kind = m.defaultKindForView(m.view)
	}
	return kind, res.Namespace, res.Name, kind != "" && res.Name != ""
}

func (m Model) defaultKindForView(view View) string {
	switch view {
	case ViewDeployments, ViewDeploymentDetail:
		return "Deployment"
	case ViewStatefulSets, ViewStatefulSetDetail:
		return "StatefulSet"
	case ViewDaemonSets, ViewDaemonSetDetail:
		return "DaemonSet"
	case ViewJobs:
		return "Job"
	case ViewCronJobs:
		return "CronJob"
	case ViewServices:
		return "Service"
	case ViewIngresses:
		return "Ingress"
	case ViewConfigMaps, ViewConfigMapDetail:
		return "ConfigMap"
	case ViewSecrets, ViewSecretDetail:
		return "Secret"
	case ViewPVCs:
		return "PersistentVolumeClaim"
	case ViewPVs:
		return "PersistentVolume"
	case ViewNamespaces, ViewNamespaceExplore:
		return "Namespace"
	case ViewNodes:
		return "Node"
	case ViewHPAs:
		return "HorizontalPodAutoscaler"
	case ViewCRDs:
		return "CustomResourceDefinition"
	case ViewEvents:
		return "Event"
	default:
		return ""
	}
}

func (m *Model) askConfirm(action confirmAction, title, message string, meta map[string]string) {
	m.confirmAction = action
	m.confirmTitle = title
	m.confirmMessage = message
	m.confirmMeta = meta
}

func (m *Model) clearConfirm() {
	m.confirmAction = confirmNone
	m.confirmTitle = ""
	m.confirmMessage = ""
	m.confirmMeta = nil
}

func (m Model) runConfirmedAction() tea.Cmd {
	meta := m.confirmMeta
	action := m.confirmAction
	switch action {
	case confirmDeletePod:
		ns, name := meta["ns"], meta["name"]
		return func() tea.Msg {
			if err := m.client.DeletePod(context.Background(), ns, name); err != nil {
				return opDoneMsg{err: err}
			}
			return opDoneMsg{status: fmt.Sprintf("Deleted pod %s/%s", ns, name), reload: true}
		}
	case confirmDeleteNamespace:
		name := meta["name"]
		return func() tea.Msg {
			if err := m.client.DeleteNamespace(context.Background(), name); err != nil {
				return opDoneMsg{err: err}
			}
			return opDoneMsg{status: fmt.Sprintf("Deleted namespace %s", name), reload: true}
		}
	case confirmRestart:
		return m.restartSelected(meta["kind"])
	case confirmScale:
		delta, _ := strconv.Atoi(meta["delta"])
		return m.scaleSelected(meta["kind"], int32(delta))
	case confirmApplyYAML:
		content := m.yamlBuffer
		return func() tea.Msg {
			msg, err := m.client.ApplyYAML(context.Background(), content)
			if err != nil {
				return opDoneMsg{err: err}
			}
			return opDoneMsg{status: msg}
		}
	}
	return nil
}

func (m Model) lintCurrentYAML(dryRun bool) tea.Cmd {
	content := m.yamlBuffer
	if content == "" && m.logViewer.Content != "" {
		content = m.logViewer.Content
	}
	return func() tea.Msg {
		issues, err := m.client.LintYAML(context.Background(), content, dryRun)
		if err != nil {
			return opDoneMsg{err: err}
		}
		if len(issues) == 0 {
			if dryRun {
				return opDoneMsg{status: "Dry-run passed"}
			}
			return opDoneMsg{status: "YAML lint clean"}
		}
		var b strings.Builder
		b.WriteString("YAML lint issues:\n")
		for _, issue := range issues {
			if issue.Line > 0 {
				fmt.Fprintf(&b, "- line %d: %s\n", issue.Line, issue.Message)
			} else {
				fmt.Fprintf(&b, "- %s\n", issue.Message)
			}
		}
		lv := components.NewLogViewer("YAML Lint")
		lv.SetSize(m.width, m.height)
		lv.SetContent(b.String())
		return execResultMsg{viewer: lv, status: fmt.Sprintf("%d lint issue(s)", len(issues))}
	}
}

func (m Model) openInteractiveShell() tea.Cmd {
	pod, ok := m.selectedPod()
	if !ok {
		return nil
	}
	ns, name := pod.Namespace, pod.Name
	container := m.selectedContainer
	args := []string{"exec", "-it", "-n", ns, name}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--", "sh", "-c", k8s.PodShellCommand)
	c := exec.Command("kubectl", args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellDoneMsg{err: err}
	})
}

func (m Model) startPortForwardWithPorts(local, remote int) tea.Cmd {
	pod, ok := m.selectedPod()
	if !ok {
		return nil
	}
	ns, name := pod.Namespace, pod.Name
	return func() tea.Msg {
		stop, err := m.client.StartPortForward(context.Background(), ns, name, local, remote)
		if err != nil {
			return portForwardMsg{err: err}
		}
		return portForwardMsg{
			stop:   stop,
			status: fmt.Sprintf("Port-forward localhost:%d → %s/%s:%d", local, ns, name, remote),
			session: portForwardSession{
				ID:        fmt.Sprintf("%s/%s:%d", ns, name, local),
				Namespace: ns,
				Pod:       name,
				Local:     local,
				Remote:    remote,
				Stop:      stop,
			},
		}
	}
}

func (m Model) scheduleWatch() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return watchTickMsg{} })
}

func (m Model) copySelectedEnvValue() tea.Cmd {
	item := m.list.SelectedItem()
	if item == nil || item.Detail == "" {
		return nil
	}
	val := strings.TrimPrefix(strings.TrimSpace(item.Detail), "= ")
	_ = clipboard.Copy(val)
	return func() tea.Msg { return components.FuzzyListCopy{Text: val} }
}

func (m Model) copyCurrentYAML() tea.Cmd {
	content := m.yamlBuffer
	if content == "" {
		content = m.logViewer.Content
	}
	if content == "" {
		return nil
	}
	_ = clipboard.Copy(content)
	return func() tea.Msg { return components.LogViewerCopy{Text: content} }
}

func renderRingLine(label string, value, total int, okTone bool) string {
	var fill int
	if total > 0 {
		fill = (value * 10) / total
		if value > 0 && fill == 0 {
			fill = 1
		}
		if fill > 10 {
			fill = 10
		}
	} else if value > 0 {
		fill = 10
	}
	bar := strings.Repeat("●", fill) + strings.Repeat("○", 10-fill)
	style := styles.MutedText
	if okTone {
		style = lipgloss.NewStyle().Foreground(styles.Success)
	} else if total > 0 && value < total {
		style = lipgloss.NewStyle().Foreground(styles.Warning)
	}
	center := fmt.Sprintf("%d", value)
	if total > 0 {
		center = fmt.Sprintf("%d/%d", value, total)
	}
	return fmt.Sprintf("  %-14s %s  %s",
		styles.NormalItem.Render(label),
		style.Render("["+bar+"]"),
		styles.Subtitle.Render(center),
	)
}

func (m Model) renderOverview() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  Cluster overview"))
	sb.WriteString("\n\n")
	if m.overviewLoading {
		sb.WriteString(styles.MutedText.Render("  Loading cluster insights…"))
		sb.WriteString("\n")
		return sb.String()
	}
	o := m.overview
	healthyPods := o.RunningPods + o.SucceededPods
	podOK := o.PendingPods == 0 && o.FailedPods == 0
	sb.WriteString(styles.MutedText.Render("  HEALTH"))
	sb.WriteString("\n")
	sb.WriteString(renderRingLine("Nodes", o.NodesReady, o.NodeCount, o.NodesReady == o.NodeCount && o.NodeCount > 0))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Pods running", o.RunningPods, o.PodCount, podOK || healthyPods == o.PodCount))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Namespaces", o.NamespaceCount, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Warnings", o.WarningEvents, 0, o.WarningEvents == 0))
	sb.WriteString("\n\n")
	sb.WriteString(styles.MutedText.Render("  WORKLOADS & NETWORK"))
	sb.WriteString("\n")
	sb.WriteString(renderRingLine("Deployments", o.Deployments, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("StatefulSets", o.StatefulSets, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("DaemonSets", o.DaemonSets, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Jobs", o.Jobs, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Services", o.Services, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("Ingresses", o.Ingresses, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("PVCs", o.PVCs, 0, true))
	sb.WriteByte('\n')
	sb.WriteString(renderRingLine("HPAs", o.HPAs, 0, true))
	sb.WriteString("\n\n")
	sb.WriteString(styles.MutedText.Render("  POD PHASES"))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Running:%d  Pending:%d  Failed:%d  Succeeded:%d\n",
		o.RunningPods, o.PendingPods, o.FailedPods, o.SucceededPods))
	sb.WriteString("\n")
	sb.WriteString(styles.MutedText.Render("  CAPACITY"))
	sb.WriteString("\n")
	sb.WriteString(components.RenderMetricsStrip4(
		fmt.Sprintf("%d nodes", o.NodeCount),
		fmt.Sprintf("%d nodes", o.NodeCount),
		fmt.Sprintf("%d nodes", o.NodeCount),
		"",
		o.CPUPercent, o.MemPercent, o.DiskPercent, nil,
		m.width,
	))
	sb.WriteByte('\n')
	return sb.String()
}

func (m Model) renderConfirm() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  " + m.confirmTitle))
	sb.WriteString("\n\n")
	sb.WriteString("  " + m.confirmMessage)
	sb.WriteString("\n\n")
	sb.WriteString(styles.HelpStyle.Render("  y:confirm  n/esc:cancel"))
	sb.WriteByte('\n')
	return sb.String()
}

func (m Model) renderNamespaceExplore() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render(fmt.Sprintf("  Namespace · %s", m.nsExploreName)))
	sb.WriteString("\n\n")
	if m.nsExploreLoading {
		sb.WriteString(styles.MutedText.Render("  Loading collections…"))
		sb.WriteString("\n")
		return sb.String()
	}
	for i, col := range m.nsExploreCollections {
		cursor := "  "
		style := styles.NormalItem
		if i == m.nsExploreCursor {
			cursor = styles.SelectedItem.Render("▸ ")
			style = styles.SelectedItem
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\n",
			cursor,
			style.Render(fmt.Sprintf("%-16s", col.Title)),
			styles.MutedText.Render(fmt.Sprintf(" %d", col.Count)),
		))
	}
	sb.WriteString("\n")
	sb.WriteString(styles.HelpStyle.Render("  ↑/↓:navigate  enter:open  y:yaml  esc:back"))
	sb.WriteByte('\n')
	return sb.String()
}

func (m Model) renderPortForwards() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  Active port-forwards"))
	sb.WriteString("\n\n")
	if len(m.portForwards) == 0 {
		sb.WriteString(styles.MutedText.Render("  No active port-forwards. From Pods press p."))
		sb.WriteString("\n")
		return sb.String()
	}
	for i, s := range m.portForwards {
		cursor := "  "
		style := styles.NormalItem
		if i == m.pfCursor {
			cursor = styles.SelectedItem.Render("▸ ")
			style = styles.SelectedItem
		}
		line := fmt.Sprintf("localhost:%d → %s/%s:%d", s.Local, s.Namespace, s.Pod, s.Remote)
		sb.WriteString(fmt.Sprintf("%s%s\n", cursor, style.Render(line)))
	}
	sb.WriteString("\n")
	sb.WriteString(styles.HelpStyle.Render("  ↑/↓:navigate  d:stop  esc:back"))
	sb.WriteByte('\n')
	return sb.String()
}

func (m Model) renderPrompt() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  " + m.promptTitle))
	sb.WriteString("\n\n  ")
	sb.WriteString(m.prompt.View())
	sb.WriteString("\n\n")
	sb.WriteString(styles.HelpStyle.Render("  enter:submit  esc:cancel"))
	sb.WriteByte('\n')
	return sb.String()
}

func newPrompt(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 40
	return ti
}
