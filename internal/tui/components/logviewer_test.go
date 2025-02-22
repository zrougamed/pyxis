package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleLogs() string {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("log line ", 3) + string(rune('A'+i%26))
	}
	return strings.Join(lines, "\n")
}

func TestNewLogViewer(t *testing.T) {
	lv := NewLogViewer("Pod Logs")
	if lv.Title != "Pod Logs" {
		t.Errorf("expected title 'Pod Logs', got %q", lv.Title)
	}
	if !lv.IsEmpty() {
		t.Error("expected empty viewer")
	}
}

func TestSetContent(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 30)
	lv.SetContent("line1\nline2\nline3")

	if lv.IsEmpty() {
		t.Error("expected non-empty after SetContent")
	}
	if len(lv.lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lv.lines))
	}
}

func TestScrollToBottom(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15) // ~9 visible rows
	lv.SetContent(sampleLogs())

	// Should have scrolled to bottom.
	vis := lv.visibleRows()
	expected := len(lv.lines) - vis
	if lv.offset != expected {
		t.Errorf("expected offset %d, got %d", expected, lv.offset)
	}
}

func TestScrollUp(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent(sampleLogs())

	startOffset := lv.offset

	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if lv.offset != startOffset-1 {
		t.Errorf("expected offset %d, got %d", startOffset-1, lv.offset)
	}
}

func TestScrollDown(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent(sampleLogs())

	// Scroll to top first.
	lv.offset = 0

	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if lv.offset != 1 {
		t.Errorf("expected offset 1, got %d", lv.offset)
	}
}

func TestScrollHome(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent(sampleLogs())

	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if lv.offset != 0 {
		t.Errorf("expected offset 0, got %d", lv.offset)
	}
}

func TestScrollEnd(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent(sampleLogs())

	lv.offset = 0
	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})

	vis := lv.visibleRows()
	expected := max(0, len(lv.lines)-vis)
	if lv.offset != expected {
		t.Errorf("expected offset %d, got %d", expected, lv.offset)
	}
}

func TestCopyEmitsLogViewerCopy(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent("important log output")

	lv, cmd := lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("expected command from copy")
	}
	msg := cmd()
	cp, ok := msg.(LogViewerCopy)
	if !ok {
		t.Fatalf("expected LogViewerCopy, got %T", msg)
	}
	if cp.Text != "important log output" {
		t.Errorf("unexpected copy text: %q", cp.Text)
	}
}

func TestCopyEmptyNoOp(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)

	_, cmd := lv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd != nil {
		t.Error("expected nil command when copying empty logs")
	}
}

func TestReset_LogViewer(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetContent("stuff")
	lv.Reset()

	if !lv.IsEmpty() {
		t.Error("expected empty after reset")
	}
	if lv.IsActive() {
		t.Error("expected inactive after reset")
	}
	if lv.offset != 0 {
		t.Errorf("expected offset 0, got %d", lv.offset)
	}
}

func TestViewRendersWithContent(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 20)
	lv.SetContent("line1\nline2")

	out := lv.View()
	if !strings.Contains(out, "line") {
		t.Error("expected view to contain log lines")
	}
}

func TestViewRendersEmpty(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 20)

	out := lv.View()
	if !strings.Contains(out, "Loading") {
		t.Error("expected loading message for empty viewer")
	}
}

func TestPageDown(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent(sampleLogs())
	lv.offset = 0

	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	vis := lv.visibleRows()
	if lv.offset != vis {
		t.Errorf("expected offset %d after PgDn, got %d", vis, lv.offset)
	}
}

func TestScrollUpAtTop(t *testing.T) {
	lv := NewLogViewer("Logs")
	lv.SetSize(80, 15)
	lv.SetContent("a\nb\nc")
	lv.offset = 0

	lv, _ = lv.Update(tea.KeyMsg{Type: tea.KeyUp})
	if lv.offset != 0 {
		t.Errorf("expected offset to remain 0, got %d", lv.offset)
	}
}
