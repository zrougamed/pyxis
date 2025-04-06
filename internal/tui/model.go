// Package tui implements the interactive terminal UI using Bubble Tea.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zrougamed/pyxis/internal/k8s"
	"github.com/zrougamed/pyxis/internal/tui/components"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// View represents the current screen in the TUI.
type View int

const (
	ViewMenu View = iota
	ViewPodList
	ViewPodLogs
	ViewPodImages
	ViewPodEnv
	ViewContextSwitch
	ViewNamespaceSwitch
	ViewDeployments
	ViewServices
	ViewNodes
	ViewEvents
	ViewSecrets
	ViewConfigMaps
	ViewStatefulSets
	ViewDaemonSets
	ViewJobs
	ViewCronJobs
	ViewIngresses
	ViewPVCs
	ViewPVs
	ViewHPAs
	ViewCRDs
	ViewHelm
	ViewContainerSelect
	// Detail views — show compact manifest in LogViewer after selecting from list.
	ViewDeploymentDetail
	ViewStatefulSetDetail
	ViewDaemonSetDetail
	ViewPodDetail
	ViewConfigMapDetail
	ViewSecretDetail
	ViewOverview
	ViewNamespaces
	ViewNamespaceExplore
	ViewYAMLDetail
	ViewPortForwards
	ViewConfirm
	ViewPrompt
)

// menuItem defines a menu entry.
type menuItem struct {
	title string
	desc  string
	view  View
}

var mainMenu = []menuItem{
	{title: "Cluster Overview", desc: "Counts, phases, and capacity", view: ViewOverview},
	{title: "Pod Logs", desc: "Fuzzy search pods and tail logs", view: ViewPodLogs},
	{title: "Pod Images", desc: "List container images (filter by status)", view: ViewPodImages},
	{title: "Pod Env Vars", desc: "Fuzzy search pods and view env variables", view: ViewPodEnv},
	{title: "Pods", desc: "List pods → compact manifest", view: ViewPodList},
	{title: "Deployments", desc: "List deployments → compact manifest", view: ViewDeployments},
	{title: "StatefulSets", desc: "List statefulsets → compact manifest", view: ViewStatefulSets},
	{title: "DaemonSets", desc: "List daemonsets → compact manifest", view: ViewDaemonSets},
	{title: "Jobs", desc: "List jobs", view: ViewJobs},
	{title: "CronJobs", desc: "List cronjobs", view: ViewCronJobs},
	{title: "Ingresses", desc: "List ingresses", view: ViewIngresses},
	{title: "PVCs", desc: "List persistent volume claims", view: ViewPVCs},
	{title: "Persistent Volumes", desc: "List cluster persistent volumes", view: ViewPVs},
	{title: "HPAs", desc: "List horizontal pod autoscalers", view: ViewHPAs},
	{title: "CRDs", desc: "List custom resource definitions", view: ViewCRDs},
	{title: "Helm Releases", desc: "List Helm releases (via secrets)", view: ViewHelm},
	{title: "Services", desc: "List services and endpoints", view: ViewServices},
	{title: "ConfigMaps", desc: "List configmaps → read contents", view: ViewConfigMaps},
	{title: "Secrets", desc: "List secrets → read contents (decoded)", view: ViewSecrets},
	{title: "Nodes", desc: "Node status with CPU/memory/disk gauges", view: ViewNodes},
	{title: "Events", desc: "Recent cluster events", view: ViewEvents},
	{title: "Namespaces", desc: "Browse and manage namespaces", view: ViewNamespaces},
	{title: "Port Forwards", desc: "Manage active port-forward sessions", view: ViewPortForwards},
	{title: "Switch Context", desc: "Change kubeconfig context", view: ViewContextSwitch},
	{title: "Switch Namespace", desc: "Change active namespace", view: ViewNamespaceSwitch},
}

// Model is the top-level Bubble Tea model.
type Model struct {
	client *k8s.Client
	view   View
	width  int
	height int
	err    error

	// Status feedback.
	statusMsg  string
	statusTime time.Time
	copied     bool
	copiedTime time.Time

	// Menu.
	menuCursor int

	// Reusable components.
	list      components.FuzzyList
	logViewer components.LogViewer

	// Data backing the list for selection lookups.
	pods          []k8s.PodInfo
	podFilter     k8s.PodPhaseFilter
	contexts      []string
	namespaces    []string
	envContainers []k8s.ContainerInfo
	resources     []k8s.ResourceInfo
	containers    []string

	// Active selections.
	selectedItem      string
	selectedNS        string
	selectedContainer string
	namespace         string

	// Log follow.
	following bool

	// Metrics sparkline history (capped).
	cpuHistory []float64
	memHistory []float64

	// Port-forward sessions.
	portForwardStop func()
	portForwards    []portForwardSession
	pfCursor        int

	// Overview / namespace explore.
	overview             overviewData
	overviewLoading      bool
	nsExploreName        string
	nsExploreCollections []nsCollection
	nsExploreLoading     bool
	nsExploreCursor      int

	// YAML session.
	yamlKind, yamlNS, yamlName, yamlBuffer string

	// Confirm / prompt overlays.
	confirmAction               confirmAction
	confirmTitle, confirmMessage string
	confirmMeta                 map[string]string
	promptMode                  string
	promptTitle                 string
	prompt                      textinput.Model
	returnView                  View

	// Watch / logs / deep-link.
	watching      bool
	logLevel      string
	logTail       int64
	pendingSearch string
}

// NewModel creates the initial TUI model.
func NewModel(client *k8s.Client, namespace string) Model {
	return Model{
		client:    client,
		view:      ViewMenu,
		namespace: namespace,
		list:      components.NewFuzzyList(""),
		logViewer: components.NewLogViewer(""),
		logLevel:  "ALL",
		logTail:   100,
	}
}

// --- Messages ---

type (
	errMsg           struct{ err error }
	podsMsg          struct{ pods []k8s.PodInfo }
	logsMsg          struct{ content string }
	envMsg           struct{ containers []k8s.ContainerInfo }
	contextsMsg      struct{ contexts []string }
	namespacesMsg    struct{ namespaces []string }
	resourcesMsg     struct{ resources []k8s.ResourceInfo }
	compactMsg       struct{ manifest *k8s.CompactManifest }
	containersMsg    struct{ names []string }
	logFollowTickMsg struct{}
	opDoneMsg        struct {
		status string
		err    error
		reload bool
	}
	portForwardMsg struct {
		stop    func()
		status  string
		err     error
		session portForwardSession
	}
	execResultMsg struct {
		viewer components.LogViewer
		status string
	}
)

// Init is defined in features.go (supports deep-linked initial views).


func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		m.logViewer.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case errMsg:
		m.err = msg.err
		return m, nil

	// --- Data messages ---
	case podsMsg:
		m.pods = msg.pods
		m.list.SetItems(m.podListItems())
		m.appendSelectedMetricsHistory()
		if m.pendingSearch != "" {
			m.list.SetFilter(m.pendingSearch)
			m.pendingSearch = ""
		}
		return m, nil

	case logsMsg:
		m.logViewer.SetLevelFilter(m.logLevel)
		m.logViewer.SetContent(msg.content)
		if m.following && m.logViewer.IsActive() {
			return m, m.scheduleLogFollow()
		}
		return m, nil

	case envMsg:
		m.envContainers = msg.containers
		m.list.SetItems(m.envListItems())
		return m, nil

	case contextsMsg:
		m.contexts = msg.contexts
		m.list.SetItems(m.contextListItems())
		return m, nil

	case namespacesMsg:
		m.namespaces = msg.namespaces
		m.list.SetItems(m.namespaceListItems())
		return m, nil

	case resourcesMsg:
		m.resources = msg.resources
		m.list.SetItems(m.resourceListItems(msg.resources))
		m.appendSelectedMetricsHistory()
		return m, nil

	case compactMsg:
		title := fmt.Sprintf("%s: %s/%s", msg.manifest.Kind, msg.manifest.Namespace, msg.manifest.Name)
		m.logViewer = components.NewLogViewer(title)
		m.logViewer.SetSize(m.width, m.height)
		m.logViewer.SetContent(msg.manifest.Content)
		return m, nil

	case containersMsg:
		if len(msg.names) > 1 {
			m.view = ViewContainerSelect
			m.containers = msg.names
			m.list.Reset()
			m.list.Title = m.viewTitle()
			m.list.SetSize(m.width, m.height)
			m.list.SetItems(m.containerListItems())
			return m, nil
		}
		container := ""
		if len(msg.names) == 1 {
			container = msg.names[0]
		}
		return m.startPodLogs(container)

	case logFollowTickMsg:
		if !m.following || !m.logViewer.IsActive() {
			return m, nil
		}
		return m, m.fetchLogs(m.selectedNS, m.selectedItem, m.selectedContainer, true)

	case opDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		if msg.reload {
			if m.view == ViewOverview {
				m.overviewLoading = true
				return m, m.loadOverview()
			}
			return m, m.loadViewData(m.view)
		}
		return m, nil

	case portForwardMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.session.Stop != nil {
			m.portForwards = append(m.portForwards, msg.session)
			m.portForwardStop = msg.session.Stop
		} else if msg.stop != nil {
			if m.portForwardStop != nil {
				m.portForwardStop()
			}
			m.portForwardStop = msg.stop
		}
		m.setStatus(msg.status)
		return m, nil

	case execResultMsg:
		m.logViewer = msg.viewer
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		return m, nil

	case overviewMsg:
		m.overviewLoading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.overview = msg.data
		return m, nil

	case nsExploreMsg:
		m.nsExploreLoading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.nsExploreName = msg.name
		m.nsExploreCollections = msg.collections
		m.nsExploreCursor = 0
		return m, nil

	case yamlMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.yamlKind, m.yamlNS, m.yamlName = msg.kind, msg.namespace, msg.name
		m.yamlBuffer = msg.content
		m.view = ViewYAMLDetail
		title := fmt.Sprintf("YAML: %s %s", msg.kind, msg.name)
		if msg.namespace != "" {
			title = fmt.Sprintf("YAML: %s %s/%s", msg.kind, msg.namespace, msg.name)
		}
		m.logViewer = components.NewLogViewer(title)
		m.logViewer.SetSize(m.width, m.height)
		m.logViewer.SetContent(msg.content)
		return m, nil

	case watchTickMsg:
		if !m.watching {
			return m, nil
		}
		if m.view == ViewOverview {
			return m, tea.Batch(m.loadOverview(), m.scheduleWatch())
		}
		return m, tea.Batch(m.loadViewData(m.view), m.scheduleWatch())

	case shellDoneMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Shell ended: %v", msg.err))
		} else {
			m.setStatus("Shell closed")
		}
		return m, nil

	// --- Component messages ---
	case components.FuzzyListSelection:
		return m.handleListSelection(msg)

	case components.FuzzyListCopy:
		m.copied = true
		m.copiedTime = time.Now()
		return m, nil

	case components.LogViewerCopy:
		m.copied = true
		m.copiedTime = time.Now()
		return m, nil
	}

	// Forward to active component.
	return m.forwardToComponent(msg)
}

func (m Model) forwardToComponent(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Detail views use the logViewer.
	if m.logViewer.IsActive() {
		var cmd tea.Cmd
		m.logViewer, cmd = m.logViewer.Update(msg)
		return m, cmd
	}
	if m.view != ViewMenu {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Confirm overlay takes priority.
	if m.confirmAction != confirmNone || m.view == ViewConfirm {
		switch key {
		case "y", "Y", "enter":
			cmd := m.runConfirmedAction()
			m.clearConfirm()
			if m.view == ViewConfirm {
				m.view = m.returnView
			}
			return m, cmd
		case "n", "N", "esc", "q":
			m.clearConfirm()
			if m.view == ViewConfirm {
				m.view = m.returnView
			}
			return m, nil
		}
		return m, nil
	}

	// Prompt overlay.
	if m.promptMode != "" || m.view == ViewPrompt {
		switch key {
		case "esc":
			m.promptMode = ""
			m.view = m.returnView
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.prompt.Value())
			mode := m.promptMode
			m.promptMode = ""
			m.view = m.returnView
			switch mode {
			case "pf-ports":
				local, remote := 8080, 8080
				parts := strings.Split(value, ":")
				if len(parts) == 2 {
					if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
						local = n
					}
					if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						remote = n
					}
				}
				return m, m.startPortForwardWithPorts(local, remote)
			case "ns-create":
				if value == "" {
					return m, nil
				}
				return m, func() tea.Msg {
					if err := m.client.CreateNamespace(context.Background(), value); err != nil {
						return opDoneMsg{err: err}
					}
					return opDoneMsg{status: fmt.Sprintf("Created namespace %s", value), reload: true}
				}
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(msg)
			return m, cmd
		}
	}

	// Namespace explorer navigation.
	if m.view == ViewNamespaceExplore {
		switch key {
		case "up", "k":
			if m.nsExploreCursor > 0 {
				m.nsExploreCursor--
			}
			return m, nil
		case "down", "j":
			if m.nsExploreCursor < len(m.nsExploreCollections)-1 {
				m.nsExploreCursor++
			}
			return m, nil
		case "enter":
			if m.nsExploreCursor >= 0 && m.nsExploreCursor < len(m.nsExploreCollections) {
				col := m.nsExploreCollections[m.nsExploreCursor]
				m.namespace = m.nsExploreName
				m.view = col.View
				m.list.Reset()
				m.list.Title = m.viewTitle()
				m.setStatus(fmt.Sprintf("Scoped to namespace %s", m.nsExploreName))
				return m, m.loadViewData(col.View)
			}
			return m, nil
		case "y":
			return m, m.fetchYAML("Namespace", "", m.nsExploreName)
		case "esc", "q":
			m.view = ViewNamespaces
			m.list.Title = m.viewTitle()
			return m, m.loadViewData(ViewNamespaces)
		}
	}

	// Port-forward manager.
	if m.view == ViewPortForwards {
		switch key {
		case "up", "k":
			if m.pfCursor > 0 {
				m.pfCursor--
			}
			return m, nil
		case "down", "j":
			if m.pfCursor < len(m.portForwards)-1 {
				m.pfCursor++
			}
			return m, nil
		case "d":
			if m.pfCursor >= 0 && m.pfCursor < len(m.portForwards) {
				s := m.portForwards[m.pfCursor]
				if s.Stop != nil {
					s.Stop()
				}
				m.portForwards = append(m.portForwards[:m.pfCursor], m.portForwards[m.pfCursor+1:]...)
				if m.pfCursor >= len(m.portForwards) && m.pfCursor > 0 {
					m.pfCursor--
				}
				m.setStatus(fmt.Sprintf("Stopped port-forward %s", s.ID))
			}
			return m, nil
		case "esc", "q":
			return m.goBack(), nil
		}
		return m, nil
	}

	// Overview is mostly static; refresh/watch only.
	if m.view == ViewOverview {
		switch key {
		case "r":
			m.overviewLoading = true
			return m, m.loadOverview()
		case "w":
			m.watching = !m.watching
			if m.watching {
				m.setStatus("Watch on")
				return m, tea.Batch(m.loadOverview(), m.scheduleWatch())
			}
			m.setStatus("Watch off")
			return m, nil
		case "esc", "q":
			m.watching = false
			return m.goBack(), nil
		}
		return m, nil
	}

	// Global keys.
	switch key {
	case "ctrl+c":
		m.stopAllPortForwards()
		return m, tea.Quit
	case "q":
		if m.list.IsSearching() {
			return m.forwardToComponent(msg)
		}
		if m.view == ViewMenu {
			m.stopAllPortForwards()
			return m, tea.Quit
		}
		return m.goBack(), nil
	case "esc":
		if m.list.IsSearching() {
			return m.forwardToComponent(msg)
		}
		if m.logViewer.IsActive() {
			m.following = false
			m.logViewer.Reset()
			switch m.view {
			case ViewDeploymentDetail:
				m.view = ViewDeployments
			case ViewStatefulSetDetail:
				m.view = ViewStatefulSets
			case ViewDaemonSetDetail:
				m.view = ViewDaemonSets
			case ViewPodDetail:
				m.view = ViewPodList
			case ViewConfigMapDetail:
				m.view = ViewConfigMaps
			case ViewSecretDetail:
				m.view = ViewSecrets
			case ViewYAMLDetail:
				m.view = m.returnView
				if m.view == ViewYAMLDetail || m.view == 0 {
					m.view = ViewMenu
				}
				m.yamlBuffer = ""
			}
			return m, nil
		}
		if m.view == ViewContainerSelect {
			m.view = ViewPodLogs
			m.list.Reset()
			m.list.Title = m.viewTitle()
			m.list.SetSize(m.width, m.height)
			m.list.SetItems(m.podListItems())
			return m, nil
		}
		if m.view != ViewMenu {
			return m.goBack(), nil
		}
		m.stopAllPortForwards()
		return m, tea.Quit
	case "f":
		if !m.list.IsSearching() && (m.view == ViewPodImages || m.view == ViewPodList) {
			m.podFilter = (m.podFilter + 1) % 6
			return m, m.loadViewData(m.view)
		}
	case "r":
		if !m.list.IsSearching() && m.view != ViewMenu {
			return m, m.loadViewData(m.view)
		}
	case "l":
		if m.logViewer.IsActive() && (m.view == ViewPodLogs || m.selectedItem != "") {
			m.following = !m.following
			if m.following {
				m.setStatus("Live logs on")
				return m, m.fetchLogs(m.selectedNS, m.selectedItem, m.selectedContainer, true)
			}
			m.setStatus("Live logs off")
			return m, nil
		}
	case "L":
		if m.logViewer.IsActive() && m.view == ViewPodLogs {
			levels := []string{"ALL", "INFO", "WARN", "ERROR", "DEBUG"}
			idx := 0
			for i, level := range levels {
				if level == m.logLevel {
					idx = (i + 1) % len(levels)
					break
				}
			}
			m.logLevel = levels[idx]
			m.logViewer.SetLevelFilter(m.logLevel)
			m.setStatus(fmt.Sprintf("Log level: %s", m.logLevel))
			return m, nil
		}
	case "w":
		if m.view != ViewMenu && !m.logViewer.IsActive() {
			m.watching = !m.watching
			if m.watching {
				m.setStatus("Watch on (3s refresh)")
				return m, tea.Batch(m.loadViewData(m.view), m.scheduleWatch())
			}
			m.setStatus("Watch off")
			return m, nil
		}
	case "y":
		if m.view == ViewYAMLDetail {
			return m, m.copyCurrentYAML()
		}
		if !m.list.IsSearching() {
			kind, ns, name, ok := m.kindForCurrentView()
			if ok {
				m.returnView = m.view
				return m, m.fetchYAML(kind, ns, name)
			}
		}
	case "Y":
		if m.view == ViewYAMLDetail {
			return m, m.lintCurrentYAML(false)
		}
	case "a":
		if m.view == ViewYAMLDetail && m.yamlBuffer != "" {
			m.askConfirm(confirmApplyYAML, "Apply YAML", "Apply the current manifest with server-side apply?", nil)
			return m, nil
		}
	case "A":
		if m.view == ViewYAMLDetail {
			return m, m.lintCurrentYAML(true)
		}
	case "C":
		if m.view == ViewPodEnv {
			return m, m.copySelectedEnvValue()
		}
		if m.view == ViewYAMLDetail {
			return m, m.copyCurrentYAML()
		}
	}

	// Route to menu or active component.
	if m.view == ViewMenu {
		return m.handleMenuKey(key)
	}

	// If logViewer is active (any detail view), forward keys there (except live ops above).
	if m.logViewer.IsActive() {
		var cmd tea.Cmd
		m.logViewer, cmd = m.logViewer.Update(msg)
		return m, cmd
	}

	// Live ops when list focused and not searching.
	if !m.list.IsSearching() {
		if cmd, handled := m.handleLiveOps(key); handled {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) handleLiveOps(key string) (tea.Cmd, bool) {
	switch key {
	case "d":
		if m.view == ViewPodList || m.view == ViewPodLogs {
			pod, ok := m.selectedPod()
			if !ok {
				return nil, true
			}
			m.askConfirm(confirmDeletePod, "Delete pod",
				fmt.Sprintf("Delete pod %s/%s?", pod.Namespace, pod.Name),
				map[string]string{"ns": pod.Namespace, "name": pod.Name})
			return nil, true
		}
		if m.view == ViewNamespaces {
			res, ok := m.selectedResource()
			if !ok {
				return nil, true
			}
			m.askConfirm(confirmDeleteNamespace, "Delete namespace",
				fmt.Sprintf("Delete namespace %s? This cannot be undone.", res.Name),
				map[string]string{"name": res.Name})
			return nil, true
		}
	case "n":
		if m.view == ViewNamespaces {
			m.returnView = m.view
			m.promptMode = "ns-create"
			m.promptTitle = "Create namespace"
			m.prompt = newPrompt("my-namespace")
			m.view = ViewPrompt
			return nil, true
		}
	case "R":
		switch m.view {
		case ViewDeployments:
			m.askConfirm(confirmRestart, "Restart deployment", "Restart selected deployment (rollout restart)?", map[string]string{"kind": "deployment"})
			return nil, true
		case ViewStatefulSets:
			m.askConfirm(confirmRestart, "Restart statefulset", "Restart selected statefulset?", map[string]string{"kind": "statefulset"})
			return nil, true
		case ViewDaemonSets:
			m.askConfirm(confirmRestart, "Restart daemonset", "Restart selected daemonset?", map[string]string{"kind": "daemonset"})
			return nil, true
		}
	case "+":
		switch m.view {
		case ViewDeployments:
			m.askConfirm(confirmScale, "Scale up", "Scale selected deployment +1?", map[string]string{"kind": "deployment", "delta": "1"})
			return nil, true
		case ViewStatefulSets:
			m.askConfirm(confirmScale, "Scale up", "Scale selected statefulset +1?", map[string]string{"kind": "statefulset", "delta": "1"})
			return nil, true
		}
	case "-":
		switch m.view {
		case ViewDeployments:
			m.askConfirm(confirmScale, "Scale down", "Scale selected deployment -1?", map[string]string{"kind": "deployment", "delta": "-1"})
			return nil, true
		case ViewStatefulSets:
			m.askConfirm(confirmScale, "Scale down", "Scale selected statefulset -1?", map[string]string{"kind": "statefulset", "delta": "-1"})
			return nil, true
		}
	case "p":
		if m.view == ViewPodList {
			m.returnView = m.view
			m.promptMode = "pf-ports"
			m.promptTitle = "Port-forward local:remote"
			m.prompt = newPrompt("8080:8080")
			m.prompt.SetValue("8080:8080")
			m.view = ViewPrompt
			return nil, true
		}
	case "x":
		if m.view == ViewPodList {
			return m.openInteractiveShell(), true
		}
	case "P":
		if m.view == ViewPodList || m.view == ViewPortForwards {
			m.view = ViewPortForwards
			return nil, true
		}
	}
	return nil, false
}

func (m Model) handleMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(mainMenu)-1 {
			m.menuCursor++
		}
	case "enter":
		selected := mainMenu[m.menuCursor]
		m.view = selected.view
		m.list.Reset()
		m.logViewer.Reset()
		m.following = false
		m.watching = false
		m.selectedContainer = ""
		m.list.Title = m.viewTitle()
		if selected.view == ViewOverview {
			m.overviewLoading = true
			return m, m.loadOverview()
		}
		if selected.view == ViewPortForwards {
			return m, nil
		}
		return m, m.loadViewData(selected.view)
	}
	return m, nil
}

func (m Model) handleListSelection(sel components.FuzzyListSelection) (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewPodLogs:
		if sel.Index < len(m.pods) {
			pod := m.pods[sel.Index]
			m.selectedItem = pod.Name
			m.selectedNS = pod.Namespace
			m.selectedContainer = ""
			return m, m.fetchPodContainers(pod.Namespace, pod.Name)
		}

	case ViewContainerSelect:
		if sel.Index < len(m.containers) {
			return m.startPodLogs(m.containers[sel.Index])
		}

	case ViewPodEnv:
		if sel.Index < len(m.pods) {
			pod := m.pods[sel.Index]
			m.selectedItem = pod.Name
			m.selectedNS = pod.Namespace
			m.list.Title = fmt.Sprintf("Env Vars: %s/%s", pod.Namespace, pod.Name)
			return m, m.fetchEnvVars(pod.Namespace, pod.Name)
		}

	case ViewContextSwitch:
		if sel.Index < len(m.contexts) {
			ctx := m.contexts[sel.Index]
			if err := m.client.SwitchContext(ctx); err != nil {
				m.err = err
			} else {
				m.setStatus(fmt.Sprintf("Switched to context: %s", ctx))
			}
			return m.goBack(), nil
		}

	case ViewNamespaceSwitch:
		if sel.Index < len(m.namespaces) {
			m.namespace = m.namespaces[sel.Index]
			m.setStatus(fmt.Sprintf("Switched to namespace: %s", m.namespace))
			return m.goBack(), nil
		}

	// --- Detail views: select from list → show compact manifest ---
	case ViewPodList:
		if sel.Index < len(m.pods) {
			pod := m.pods[sel.Index]
			m.view = ViewPodDetail
			return m, m.fetchCompact("Pod", pod.Namespace, pod.Name)
		}

	case ViewDeployments:
		return m, m.fetchCompactFromSelection(sel, "Deployment", ViewDeploymentDetail)

	case ViewStatefulSets:
		return m, m.fetchCompactFromSelection(sel, "StatefulSet", ViewStatefulSetDetail)

	case ViewDaemonSets:
		return m, m.fetchCompactFromSelection(sel, "DaemonSet", ViewDaemonSetDetail)

	case ViewConfigMaps:
		return m, m.fetchCompactFromSelection(sel, "ConfigMap", ViewConfigMapDetail)

	case ViewSecrets:
		return m, m.fetchCompactFromSelection(sel, "Secret", ViewSecretDetail)

	case ViewNamespaces:
		if sel.Index < len(m.resources) {
			ns := m.resources[sel.Index].Name
			m.view = ViewNamespaceExplore
			m.nsExploreName = ns
			m.nsExploreLoading = true
			m.nsExploreCollections = nil
			return m, m.loadNamespaceExplore(ns)
		}
	}
	return m, nil
}

func (m Model) startPodLogs(container string) (Model, tea.Cmd) {
	m.view = ViewPodLogs
	m.selectedContainer = container
	m.following = true
	title := fmt.Sprintf("Logs: %s/%s", m.selectedNS, m.selectedItem)
	if container != "" {
		title = fmt.Sprintf("Logs: %s/%s [%s]", m.selectedNS, m.selectedItem, container)
	}
	m.logViewer = components.NewLogViewer(title)
	m.logViewer.SetSize(m.width, m.height)
	return m, m.fetchLogs(m.selectedNS, m.selectedItem, container, true)
}

// fetchCompactFromSelection extracts namespace/name from the selected item ID
// (format: "namespace/name") and fetches the compact manifest.
func (m *Model) fetchCompactFromSelection(sel components.FuzzyListSelection, kind string, detailView View) tea.Cmd {
	ns, name := splitNsName(sel.Item.ID)
	if ns == "" {
		ns = m.namespace
	}
	m.view = detailView
	return m.fetchCompact(kind, ns, name)
}

func (m Model) goBack() Model {
	m.following = false
	m.watching = false
	m.clearConfirm()
	m.promptMode = ""
	m.view = ViewMenu
	m.list.Reset()
	m.logViewer.Reset()
	m.err = nil
	m.selectedItem = ""
	m.selectedNS = ""
	m.selectedContainer = ""
	m.containers = nil
	m.yamlBuffer = ""
	return m
}

func (m *Model) setStatus(msg string) {
	m.statusMsg = msg
	m.statusTime = time.Now()
}

func (m *Model) stopPortForward() {
	if m.portForwardStop != nil {
		m.portForwardStop()
		m.portForwardStop = nil
	}
}

func (m *Model) stopAllPortForwards() {
	m.stopPortForward()
	for _, s := range m.portForwards {
		if s.Stop != nil {
			s.Stop()
		}
	}
	m.portForwards = nil
}

func (m *Model) appendSelectedMetricsHistory() {
	cpu, mem, ok := m.selectedMetricsPercents()
	if !ok {
		return
	}
	m.cpuHistory = appendFloatHistory(m.cpuHistory, cpu, 20)
	m.memHistory = appendFloatHistory(m.memHistory, mem, 20)
}

func (m Model) selectedMetricsPercents() (cpu, mem float64, ok bool) {
	switch m.view {
	case ViewPodList, ViewPodLogs, ViewPodEnv:
		item := m.list.SelectedItem()
		if item == nil {
			return 0, 0, false
		}
		idx, err := strconv.Atoi(item.ID)
		if err != nil || idx < 0 || idx >= len(m.pods) {
			return 0, 0, false
		}
		pod := m.pods[idx]
		if pod.CPULabel == "" && pod.MemoryLabel == "" {
			return 0, 0, false
		}
		return derefPercent(pod.CPUPercent), derefPercent(pod.MemoryPercent), true
	case ViewNodes:
		item := m.list.SelectedItem()
		if item == nil {
			return 0, 0, false
		}
		for _, r := range m.resources {
			if r.Name == item.ID || r.Name == item.Name {
				if r.CPULabel == "" && r.MemoryLabel == "" {
					return 0, 0, false
				}
				return derefPercent(r.CPUPercent), derefPercent(r.MemoryPercent), true
			}
		}
	}
	return 0, 0, false
}

func derefPercent(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func appendFloatHistory(hist []float64, v float64, capN int) []float64 {
	hist = append(hist, v)
	if len(hist) > capN {
		hist = hist[len(hist)-capN:]
	}
	return hist
}

// --- Live ops helpers ---

func (m Model) selectedPod() (k8s.PodInfo, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return k8s.PodInfo{}, false
	}
	idx, err := strconv.Atoi(item.ID)
	if err != nil || idx < 0 || idx >= len(m.pods) {
		return k8s.PodInfo{}, false
	}
	return m.pods[idx], true
}

func (m Model) selectedResource() (k8s.ResourceInfo, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return k8s.ResourceInfo{}, false
	}
	ns, name := splitNsName(item.ID)
	for _, r := range m.resources {
		if r.Name == name && (ns == "" || r.Namespace == ns) {
			return r, true
		}
		if item.ID == r.Name || item.Name == r.Name {
			return r, true
		}
	}
	return k8s.ResourceInfo{}, false
}

func (m Model) deleteSelectedPod() tea.Cmd {
	pod, ok := m.selectedPod()
	if !ok {
		return nil
	}
	ns, name := pod.Namespace, pod.Name
	return func() tea.Msg {
		ctx := context.Background()
		if err := m.client.DeletePod(ctx, ns, name); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{status: fmt.Sprintf("Deleted pod %s/%s", ns, name), reload: true}
	}
}

func (m Model) restartSelected(kind string) tea.Cmd {
	res, ok := m.selectedResource()
	if !ok {
		return nil
	}
	ns, name := res.Namespace, res.Name
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		switch kind {
		case "deployment":
			err = m.client.RestartDeployment(ctx, ns, name)
		case "statefulset":
			err = m.client.RestartStatefulSet(ctx, ns, name)
		case "daemonset":
			err = m.client.RestartDaemonSet(ctx, ns, name)
		}
		if err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{status: fmt.Sprintf("Restarted %s %s/%s", kind, ns, name), reload: true}
	}
}

func (m Model) scaleSelected(kind string, delta int32) tea.Cmd {
	res, ok := m.selectedResource()
	if !ok {
		return nil
	}
	desired, ok := parseDesiredReplicas(res.Status)
	if !ok {
		if ready, okReady := res.Extra["ready"]; okReady {
			desired, ok = parseDesiredReplicas(ready)
		}
	}
	if !ok {
		return func() tea.Msg {
			return opDoneMsg{status: "Cannot determine replica count"}
		}
	}
	next := desired + delta
	if next < 0 {
		next = 0
	}
	ns, name := res.Namespace, res.Name
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		switch kind {
		case "deployment":
			err = m.client.ScaleDeployment(ctx, ns, name, next)
		case "statefulset":
			err = m.client.ScaleStatefulSet(ctx, ns, name, next)
		}
		if err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{status: fmt.Sprintf("Scaled %s %s/%s to %d", kind, ns, name, next), reload: true}
	}
}

func parseDesiredReplicas(status string) (int32, bool) {
	// Formats: "3/3 ready", "1/1", or Extra ready values.
	status = strings.TrimSpace(status)
	parts := strings.Fields(status)
	if len(parts) == 0 {
		return 0, false
	}
	frac := parts[0]
	slash := strings.IndexByte(frac, '/')
	if slash < 0 {
		n, err := strconv.Atoi(frac)
		if err != nil {
			return 0, false
		}
		return int32(n), true
	}
	desired, err := strconv.Atoi(frac[slash+1:])
	if err != nil {
		return 0, false
	}
	return int32(desired), true
}

func (m Model) portForwardSelected() tea.Cmd {
	pod, ok := m.selectedPod()
	if !ok {
		return nil
	}
	ns, name := pod.Namespace, pod.Name
	return func() tea.Msg {
		ctx := context.Background()
		stop, err := m.client.StartPortForward(ctx, ns, name, 8080, 8080)
		if err != nil {
			return portForwardMsg{err: err}
		}
		return portForwardMsg{
			stop:   stop,
			status: fmt.Sprintf("Port-forward 8080→8080 on %s/%s", ns, name),
		}
	}
}

func (m Model) execSelected() tea.Cmd {
	pod, ok := m.selectedPod()
	if !ok {
		return nil
	}
	ns, name := pod.Namespace, pod.Name
	container := m.selectedContainer
	width, height := m.width, m.height
	return func() tea.Msg {
		ctx := context.Background()
		var buf strings.Builder
		err := m.client.ExecInPod(ctx, ns, name, container, []string{"sh", "-c", "hostname; id"}, &buf, &buf, nil)
		if err != nil {
			return opDoneMsg{err: err}
		}
		title := fmt.Sprintf("Exec: %s/%s", ns, name)
		lv := components.NewLogViewer(title)
		lv.SetSize(width, height)
		lv.SetContent(buf.String())
		return execResultMsg{viewer: lv, status: fmt.Sprintf("Exec completed on %s/%s", ns, name)}
	}
}

// --- Data loading commands ---

func (m Model) loadViewData(view View) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		switch view {
		case ViewPodLogs, ViewPodList, ViewPodEnv, ViewPodImages:
			pods, err := m.client.ListPods(ctx, m.namespace, m.podFilter)
			if err != nil {
				return errMsg{err}
			}
			return podsMsg{pods}

		case ViewContextSwitch:
			return contextsMsg{m.client.Contexts()}

		case ViewNamespaceSwitch:
			ns, err := m.client.Namespaces(ctx)
			if err != nil {
				return errMsg{err}
			}
			return namespacesMsg{ns}

		case ViewDeployments:
			res, err := m.client.ListDeployments(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewServices:
			res, err := m.client.ListServices(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewNodes:
			res, err := m.client.ListNodes(ctx)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewEvents:
			res, err := m.client.ListEvents(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewConfigMaps:
			res, err := m.client.ListConfigMaps(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewSecrets:
			res, err := m.client.ListSecrets(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewStatefulSets:
			res, err := m.client.ListStatefulSets(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewDaemonSets:
			res, err := m.client.ListDaemonSets(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewJobs:
			res, err := m.client.ListJobs(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewCronJobs:
			res, err := m.client.ListCronJobs(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewIngresses:
			res, err := m.client.ListIngresses(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewPVCs:
			res, err := m.client.ListPersistentVolumeClaims(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewPVs:
			res, err := m.client.ListPersistentVolumes(ctx)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewNamespaces:
			res, err := m.client.ListNamespaceInfos(ctx)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewHPAs:
			res, err := m.client.ListHPAs(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewCRDs:
			res, err := m.client.ListCRDs(ctx)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}

		case ViewHelm:
			res, err := m.client.ListHelmReleases(ctx, m.namespace)
			if err != nil {
				return errMsg{err}
			}
			return resourcesMsg{res}
		}
		return nil
	}
}

func (m Model) fetchPodContainers(namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		names, err := m.client.GetPodContainers(ctx, namespace, podName)
		if err != nil {
			return errMsg{err}
		}
		return containersMsg{names}
	}
}

func (m Model) fetchLogs(namespace, podName, container string, follow bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		tail := int64(100)
		if follow {
			tail = 200
		}
		stream, err := m.client.GetPodLogs(ctx, namespace, podName, container, tail, false)
		if err != nil {
			return errMsg{err}
		}
		defer stream.Close()

		var buf strings.Builder
		data := make([]byte, 4096)
		for {
			n, readErr := stream.Read(data)
			if n > 0 {
				buf.Write(data[:n])
			}
			if readErr != nil {
				break
			}
		}
		return logsMsg{buf.String()}
	}
}

func (m Model) scheduleLogFollow() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return logFollowTickMsg{}
	})
}

func (m Model) fetchEnvVars(namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		containers, err := m.client.GetPodEnvVars(ctx, namespace, podName)
		if err != nil {
			return errMsg{err}
		}
		return envMsg{containers}
	}
}

func (m Model) fetchCompact(kind, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var manifest *k8s.CompactManifest
		var err error

		switch kind {
		case "Pod":
			manifest, err = m.client.GetPodCompact(ctx, namespace, name)
		case "Deployment":
			manifest, err = m.client.GetDeploymentCompact(ctx, namespace, name)
		case "StatefulSet":
			manifest, err = m.client.GetStatefulSetCompact(ctx, namespace, name)
		case "DaemonSet":
			manifest, err = m.client.GetDaemonSetCompact(ctx, namespace, name)
		case "ConfigMap":
			manifest, err = m.client.GetConfigMapData(ctx, namespace, name)
		case "Secret":
			manifest, err = m.client.GetSecretData(ctx, namespace, name)
		default:
			return errMsg{fmt.Errorf("unknown kind %q", kind)}
		}

		if err != nil {
			return errMsg{err}
		}
		return compactMsg{manifest}
	}
}

// splitNsName splits "namespace/name" into its parts.
// If there's no slash, returns ("", input).
func splitNsName(s string) (string, string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", s
}

// --- Item builders: translate domain data into FuzzyListItems ---

func (m Model) podListItems() []components.FuzzyListItem {
	items := make([]components.FuzzyListItem, len(m.pods))
	for i, pod := range m.pods {
		name := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		var detail string
		switch m.view {
		case ViewPodImages:
			detail = fmt.Sprintf("  %s  %s",
				strings.Join(pod.Images, ", "),
				styles.PhaseStyle(string(pod.Phase)).Render(string(pod.Phase)))
		default:
			detail = fmt.Sprintf("  %s  Ready:%s  Restarts:%d  Node:%s%s",
				styles.PhaseStyle(string(pod.Phase)).Render(string(pod.Phase)),
				pod.Ready, pod.Restarts, pod.Node,
				components.RenderMetricsInline4(pod.CPULabel, pod.MemoryLabel, pod.DiskLabel, pod.NetworkLabel, pod.CPUPercent, pod.MemoryPercent, pod.DiskPercent, pod.NetworkPercent))
		}
		items[i] = components.FuzzyListItem{
			ID:     fmt.Sprintf("%d", i),
			Name:   name,
			Detail: detail,
		}
	}
	return items
}

func (m Model) containerListItems() []components.FuzzyListItem {
	items := make([]components.FuzzyListItem, len(m.containers))
	for i, name := range m.containers {
		items[i] = components.FuzzyListItem{
			ID:   name,
			Name: name,
		}
	}
	return items
}

func (m Model) envListItems() []components.FuzzyListItem {
	var items []components.FuzzyListItem
	for _, c := range m.envContainers {
		items = append(items, components.FuzzyListItem{
			Name:   fmt.Sprintf("── Container: %s (%s)", c.Name, c.Image),
			Detail: "",
		})
		for _, env := range c.EnvVars {
			val := env.Value
			if val == "" {
				val = env.ValueFrom
			}
			items = append(items, components.FuzzyListItem{
				Name:   fmt.Sprintf("  %s", env.Name),
				Detail: fmt.Sprintf("  = %s", val),
			})
		}
	}
	return items
}

func (m Model) contextListItems() []components.FuzzyListItem {
	items := make([]components.FuzzyListItem, len(m.contexts))
	for i, ctx := range m.contexts {
		detail := ""
		if ctx == m.client.CurrentContext() {
			detail = " (current)"
		}
		items[i] = components.FuzzyListItem{ID: ctx, Name: ctx, Detail: detail}
	}
	return items
}

func (m Model) namespaceListItems() []components.FuzzyListItem {
	items := make([]components.FuzzyListItem, len(m.namespaces))
	for i, ns := range m.namespaces {
		detail := ""
		if ns == m.namespace {
			detail = " (current)"
		}
		items[i] = components.FuzzyListItem{ID: ns, Name: ns, Detail: detail}
	}
	return items
}

func (m Model) resourceListItems(resources []k8s.ResourceInfo) []components.FuzzyListItem {
	items := make([]components.FuzzyListItem, len(resources))
	for i, r := range resources {
		name := r.Name
		if r.Namespace != "" {
			name = fmt.Sprintf("%s/%s", r.Namespace, r.Name)
		}
		var extras []string
		extras = append(extras, r.Status)
		for k, v := range r.Extra {
			extras = append(extras, fmt.Sprintf("%s:%s", k, v))
		}
		detail := fmt.Sprintf("  %s", strings.Join(extras, "  "))
		detail += components.RenderMetricsInline4(r.CPULabel, r.MemoryLabel, r.DiskLabel, r.NetworkLabel, r.CPUPercent, r.MemoryPercent, r.DiskPercent, r.NetworkPercent)
		items[i] = components.FuzzyListItem{
			ID:     name,
			Name:   name,
			Detail: detail,
		}
	}
	return items
}

// --- View titles ---

func (m Model) viewTitle() string {
	switch m.view {
	case ViewPodList:
		return "Pods"
	case ViewPodLogs:
		return "Select Pod for Logs"
	case ViewPodImages:
		return "Container Images"
	case ViewPodEnv:
		return "Select Pod for Env Vars"
	case ViewContainerSelect:
		return "Select Container"
	case ViewContextSwitch:
		return "Switch Context"
	case ViewNamespaceSwitch:
		return "Switch Namespace"
	case ViewDeployments:
		return "Deployments"
	case ViewServices:
		return "Services"
	case ViewNodes:
		return "Nodes"
	case ViewEvents:
		return "Events"
	case ViewConfigMaps:
		return "ConfigMaps"
	case ViewSecrets:
		return "Secrets"
	case ViewStatefulSets:
		return "StatefulSets"
	case ViewDaemonSets:
		return "DaemonSets"
	case ViewJobs:
		return "Jobs"
	case ViewCronJobs:
		return "CronJobs"
	case ViewIngresses:
		return "Ingresses"
	case ViewPVCs:
		return "PVCs"
	case ViewPVs:
		return "Persistent Volumes"
	case ViewNamespaces:
		return "Namespaces"
	case ViewNamespaceExplore:
		return "Namespace explorer"
	case ViewOverview:
		return "Cluster Overview"
	case ViewYAMLDetail:
		return "YAML"
	case ViewPortForwards:
		return "Port Forwards"
	case ViewHPAs:
		return "HPAs"
	case ViewCRDs:
		return "CRDs"
	case ViewHelm:
		return "Helm Releases"
	default:
		return ""
	}
}

// FilterName returns the human-readable pod filter name.
func FilterName(f k8s.PodPhaseFilter) string {
	switch f {
	case k8s.PodFilterAll:
		return "All"
	case k8s.PodFilterRunning:
		return "Running"
	case k8s.PodFilterNotRunning:
		return "Not Running"
	case k8s.PodFilterFailed:
		return "Failed"
	case k8s.PodFilterPending:
		return "Pending"
	case k8s.PodFilterSucceeded:
		return "Succeeded"
	default:
		return "All"
	}
}
