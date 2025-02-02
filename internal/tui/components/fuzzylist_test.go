package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleItems() []FuzzyListItem {
	return []FuzzyListItem{
		{ID: "1", Name: "default/nginx-7d4b8c-abc12", Detail: "  Running"},
		{ID: "2", Name: "default/redis-cache-5f8c3a", Detail: "  Pending"},
		{ID: "3", Name: "production/api-server-9x8z7", Detail: "  Running"},
		{ID: "4", Name: "staging/nginx-ingress-ctrl", Detail: "  Failed"},
	}
}

func TestNewFuzzyList(t *testing.T) {
	fl := NewFuzzyList("Test List")
	if fl.Title != "Test List" {
		t.Errorf("expected title 'Test List', got %q", fl.Title)
	}
	if fl.IsSearching() {
		t.Error("should not be searching initially")
	}
}

func TestSetItems(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())

	if len(fl.Items) != 4 {
		t.Errorf("expected 4 items, got %d", len(fl.Items))
	}
	if len(fl.filtered) != 4 {
		t.Errorf("expected 4 filtered indices, got %d", len(fl.filtered))
	}
	if fl.cursor != 0 {
		t.Errorf("cursor should be 0, got %d", fl.cursor)
	}
}

func TestSelectedItem(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())

	item := fl.SelectedItem()
	if item == nil {
		t.Fatal("expected a selected item")
	}
	if item.ID != "1" {
		t.Errorf("expected ID '1', got %q", item.ID)
	}
}

func TestSelectedItem_Empty(t *testing.T) {
	fl := NewFuzzyList("Empty")
	item := fl.SelectedItem()
	if item != nil {
		t.Error("expected nil for empty list")
	}
}

func TestMoveDown(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	fl.moveDown()
	if fl.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", fl.cursor)
	}

	fl.moveDown()
	fl.moveDown()
	if fl.cursor != 3 {
		t.Errorf("expected cursor 3, got %d", fl.cursor)
	}

	// Should not go past end.
	fl.moveDown()
	if fl.cursor != 3 {
		t.Errorf("expected cursor to stay at 3, got %d", fl.cursor)
	}
}

func TestMoveUp(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	// At position 0, should not go negative.
	fl.moveUp()
	if fl.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", fl.cursor)
	}

	fl.moveDown()
	fl.moveDown()
	fl.moveUp()
	if fl.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", fl.cursor)
	}
}

func TestSearchActivation(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	// Press '/' to search.
	fl2, _ := fl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !fl2.IsSearching() {
		t.Error("expected search mode to be active")
	}

	// Press Esc to cancel.
	fl3, _ := fl2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if fl3.IsSearching() {
		t.Error("expected search mode to be cancelled")
	}
}

func TestFilterApplied(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	// Activate search.
	fl, _ = fl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	// Type "nginx".
	for _, r := range "nginx" {
		fl, _ = fl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if len(fl.filtered) != 2 {
		t.Errorf("expected 2 matches for 'nginx', got %d", len(fl.filtered))
	}
}

func TestEnterEmitsSelection(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	fl, cmd := fl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command from Enter")
	}

	msg := cmd()
	sel, ok := msg.(FuzzyListSelection)
	if !ok {
		t.Fatalf("expected FuzzyListSelection, got %T", msg)
	}
	if sel.Item.ID != "1" {
		t.Errorf("expected selected ID '1', got %q", sel.Item.ID)
	}
}

func TestCopyEmitsMessage(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	fl, cmd := fl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("expected a command from 'c'")
	}

	msg := cmd()
	cp, ok := msg.(FuzzyListCopy)
	if !ok {
		t.Fatalf("expected FuzzyListCopy, got %T", msg)
	}
	if cp.Text == "" {
		t.Error("expected non-empty copy text")
	}
}

func TestReset(t *testing.T) {
	fl := NewFuzzyList("Pods")
	fl.SetItems(sampleItems())
	fl.moveDown()
	fl.moveDown()

	fl.Reset()

	if len(fl.Items) != 0 {
		t.Errorf("expected 0 items after reset, got %d", len(fl.Items))
	}
	if fl.cursor != 0 {
		t.Errorf("expected cursor 0 after reset, got %d", fl.cursor)
	}
	if fl.IsSearching() {
		t.Error("should not be searching after reset")
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	fl := NewFuzzyList("Test")
	fl.SetItems(sampleItems())
	fl.SetSize(80, 40)

	out := fl.View()
	if out == "" {
		t.Error("expected non-empty view output")
	}
}

func TestViewEmptyList(t *testing.T) {
	fl := NewFuzzyList("Empty")
	fl.SetItems(nil)
	fl.SetSize(80, 40)

	out := fl.View()
	if out == "" {
		t.Error("expected non-empty view even with no items")
	}
}

func TestScrollAdjustment(t *testing.T) {
	fl := NewFuzzyList("Scroll Test")
	items := make([]FuzzyListItem, 100)
	for i := range items {
		items[i] = FuzzyListItem{ID: string(rune(i)), Name: "item-" + string(rune('A'+i%26))}
	}
	fl.SetItems(items)
	fl.SetSize(80, 15) // Only ~7 visible rows.

	// Move cursor well past visible area.
	for range 20 {
		fl.moveDown()
	}

	if fl.offset == 0 {
		t.Error("expected offset to have scrolled")
	}
	if fl.cursor != 20 {
		t.Errorf("expected cursor at 20, got %d", fl.cursor)
	}
}

func TestMakeRange(t *testing.T) {
	r := makeRange(5)
	if len(r) != 5 {
		t.Fatalf("expected 5, got %d", len(r))
	}
	for i, v := range r {
		if i != v {
			t.Errorf("r[%d] = %d, want %d", i, v, i)
		}
	}

	r0 := makeRange(0)
	if len(r0) != 0 {
		t.Errorf("expected empty, got %d", len(r0))
	}
}
