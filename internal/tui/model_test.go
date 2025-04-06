package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/zrougamed/pyxis/internal/k8s"
	"github.com/zrougamed/pyxis/internal/tui/components"
)

func newTestModel() Model {
	cs := fake.NewSimpleClientset()
	client := k8s.NewClientWithInterface(cs)
	return NewModel(client, "default")
}

func TestNewModel(t *testing.T) {
	m := newTestModel()

	if m.view != ViewMenu {
		t.Errorf("expected initial view to be ViewMenu, got %v", m.view)
	}
	if m.namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", m.namespace)
	}
}

func TestMenuNavigation(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	// Navigate down.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model := m2.(Model)
	if model.menuCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", model.menuCursor)
	}

	// Navigate up.
	m3, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	model = m3.(Model)
	if model.menuCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", model.menuCursor)
	}

	// Don't go below 0.
	m4, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	model = m4.(Model)
	if model.menuCursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", model.menuCursor)
	}
}

func TestEscFromSubview(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.view = ViewPodList

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := m2.(Model)
	if model.view != ViewMenu {
		t.Errorf("expected to return to menu, got view %v", model.view)
	}
}

func TestQuitFromMenu(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestWindowResize(t *testing.T) {
	m := newTestModel()
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	model := m2.(Model)
	if model.width != 120 || model.height != 50 {
		t.Errorf("expected 120x50, got %dx%d", model.width, model.height)
	}
}

func TestPodListItemsBuilder(t *testing.T) {
	m := newTestModel()
	m.view = ViewPodList
	m.pods = []k8s.PodInfo{
		{Name: "nginx", Namespace: "default", Phase: "Running", Ready: "1/1"},
		{Name: "redis", Namespace: "default", Phase: "Pending", Ready: "0/1"},
	}
	items := m.podListItems()

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "default/nginx" {
		t.Errorf("expected 'default/nginx', got %q", items[0].Name)
	}
}

func TestPodImageItemsBuilder(t *testing.T) {
	m := newTestModel()
	m.view = ViewPodImages
	m.pods = []k8s.PodInfo{
		{Name: "nginx", Namespace: "default", Phase: "Running", Images: []string{"nginx:1.25"}},
	}
	items := m.podListItems()

	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	// Detail should contain the image name.
	if items[0].Detail == "" {
		t.Error("expected non-empty detail for image view")
	}
}

func TestEnvListItemsBuilder(t *testing.T) {
	m := newTestModel()
	m.envContainers = []k8s.ContainerInfo{
		{
			Name:  "app",
			Image: "myapp:v1",
			EnvVars: []k8s.EnvVar{
				{Name: "DB_HOST", Value: "localhost"},
				{Name: "SECRET", ValueFrom: "secret:db/pass"},
			},
		},
	}
	items := m.envListItems()

	// 1 header + 2 env vars = 3.
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestContextListItemsBuilder(t *testing.T) {
	m := newTestModel()
	m.contexts = []string{"dev", "staging", "test-context"}
	items := m.contextListItems()

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	// "test-context" is the current context for the test client.
	if items[2].Detail != " (current)" {
		t.Errorf("expected current marker on test-context, got %q", items[2].Detail)
	}
}

func TestNamespaceListItemsBuilder(t *testing.T) {
	m := newTestModel()
	m.namespace = "production"
	m.namespaces = []string{"default", "production", "staging"}
	items := m.namespaceListItems()

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items[1].Detail != " (current)" {
		t.Errorf("expected current marker on production, got %q", items[1].Detail)
	}
}

func TestResourceListItemsBuilder(t *testing.T) {
	m := newTestModel()
	resources := []k8s.ResourceInfo{
		{Kind: "Deployment", Name: "nginx", Namespace: "default", Status: "3/3 ready"},
		{Kind: "Node", Name: "worker-1", Namespace: "", Status: "Ready"},
	}
	items := m.resourceListItems(resources)

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "default/nginx" {
		t.Errorf("expected 'default/nginx', got %q", items[0].Name)
	}
	if items[1].Name != "worker-1" {
		t.Errorf("expected 'worker-1' (no namespace), got %q", items[1].Name)
	}
}

func TestFilterName(t *testing.T) {
	tests := []struct {
		filter k8s.PodPhaseFilter
		want   string
	}{
		{k8s.PodFilterAll, "All"},
		{k8s.PodFilterRunning, "Running"},
		{k8s.PodFilterNotRunning, "Not Running"},
		{k8s.PodFilterFailed, "Failed"},
		{k8s.PodFilterPending, "Pending"},
		{k8s.PodFilterSucceeded, "Succeeded"},
	}

	for _, tt := range tests {
		got := FilterName(tt.filter)
		if got != tt.want {
			t.Errorf("FilterName(%d) = %q, want %q", tt.filter, got, tt.want)
		}
	}
}

func TestHandleListSelection_ContextSwitch(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.view = ViewContextSwitch
	m.contexts = []string{"dev", "staging"}

	// Simulate selecting "dev".
	sel := components.FuzzyListSelection{Index: 0, Item: components.FuzzyListItem{ID: "dev", Name: "dev"}}
	m2, _ := m.Update(sel)
	model := m2.(Model)

	// Should return to menu after context switch (even if it errors with fake client).
	if model.view != ViewMenu {
		t.Errorf("expected return to menu after context switch, got %v", model.view)
	}
}

func TestHandleListSelection_NamespaceSwitch(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.view = ViewNamespaceSwitch
	m.namespaces = []string{"default", "production"}

	sel := components.FuzzyListSelection{Index: 1, Item: components.FuzzyListItem{ID: "production", Name: "production"}}
	m2, _ := m.Update(sel)
	model := m2.(Model)

	if model.namespace != "production" {
		t.Errorf("expected namespace 'production', got %q", model.namespace)
	}
	if model.view != ViewMenu {
		t.Errorf("expected return to menu, got %v", model.view)
	}
}

func TestGoBack(t *testing.T) {
	m := newTestModel()
	m.view = ViewDeployments
	m.selectedItem = "something"

	m = m.goBack()

	if m.view != ViewMenu {
		t.Errorf("expected ViewMenu, got %v", m.view)
	}
	if m.selectedItem != "" {
		t.Errorf("expected empty selectedItem, got %q", m.selectedItem)
	}
}

func TestViewTitle(t *testing.T) {
	m := newTestModel()

	tests := []struct {
		view View
		want string
	}{
		{ViewPodList, "Pods"},
		{ViewPodLogs, "Select Pod for Logs"},
		{ViewDeployments, "Deployments"},
		{ViewNodes, "Nodes"},
		{ViewContextSwitch, "Switch Context"},
		{ViewJobs, "Jobs"},
		{ViewCronJobs, "CronJobs"},
		{ViewIngresses, "Ingresses"},
		{ViewPVCs, "PVCs"},
		{ViewHPAs, "HPAs"},
		{ViewCRDs, "CRDs"},
		{ViewHelm, "Helm Releases"},
		{ViewContainerSelect, "Select Container"},
	}

	for _, tt := range tests {
		m.view = tt.view
		got := m.viewTitle()
		if got != tt.want {
			t.Errorf("viewTitle(%v) = %q, want %q", tt.view, got, tt.want)
		}
	}
}

func TestParseDesiredReplicas(t *testing.T) {
	tests := []struct {
		in   string
		want int32
		ok   bool
	}{
		{"3/3 ready", 3, true},
		{"0/1 ready", 1, true},
		{"5/5", 5, true},
		{"", 0, false},
		{"Ready", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDesiredReplicas(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseDesiredReplicas(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIsActiveRouting(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.view = ViewPodLogs
	m.logViewer = components.NewLogViewer("Logs: default/nginx")
	// Title set but empty content — IsActive should still route to viewer.
	if !m.logViewer.IsActive() {
		t.Fatal("expected log viewer active")
	}
	if !m.logViewer.IsEmpty() {
		t.Fatal("expected empty content")
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model := m2.(Model)
	if model.logViewer.IsActive() {
		t.Error("expected log viewer inactive after esc")
	}
	if model.following {
		t.Error("expected following stopped after esc")
	}
}
