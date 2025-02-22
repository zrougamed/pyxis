package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zrougamed/pyxis/internal/clipboard"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// LogViewerCopy is emitted when the user copies log content.
type LogViewerCopy struct {
	Text string
}

// LogViewer displays scrollable log output.
type LogViewer struct {
	Title   string
	Content string

	lines       []string
	offset      int
	height      int
	width       int
	levelFilter string
}

// NewLogViewer creates a LogViewer with a title.
func NewLogViewer(title string) LogViewer {
	return LogViewer{Title: title, levelFilter: "ALL"}
}

// SetLevelFilter restricts visible lines by log level (ALL/INFO/WARN/ERROR/DEBUG).
func (lv *LogViewer) SetLevelFilter(level string) {
	if level == "" {
		level = "ALL"
	}
	lv.levelFilter = strings.ToUpper(level)
	if lv.Content != "" {
		lv.rebuildLines()
	}
}

func (lv *LogViewer) rebuildLines() {
	raw := strings.Split(lv.Content, "\n")
	if lv.levelFilter == "" || lv.levelFilter == "ALL" {
		lv.lines = raw
		return
	}
	filtered := make([]string, 0, len(raw))
	for _, line := range raw {
		if lineMatchesLevel(line, lv.levelFilter) {
			filtered = append(filtered, line)
		}
	}
	lv.lines = filtered
}

func lineMatchesLevel(line, level string) bool {
	upper := strings.ToUpper(line)
	switch level {
	case "ERROR":
		return strings.Contains(upper, "ERROR") || strings.Contains(upper, `"LEVEL":"ERROR"`) || strings.Contains(upper, "LEVEL=ERROR")
	case "WARN":
		return strings.Contains(upper, "WARN") || strings.Contains(upper, `"LEVEL":"WARN"`) || strings.Contains(upper, "LEVEL=WARN")
	case "INFO":
		return strings.Contains(upper, "INFO") || strings.Contains(upper, `"LEVEL":"INFO"`) || strings.Contains(upper, "LEVEL=INFO")
	case "DEBUG":
		return strings.Contains(upper, "DEBUG") || strings.Contains(upper, `"LEVEL":"DEBUG"`) || strings.Contains(upper, "LEVEL=DEBUG")
	default:
		return true
	}
}

// SetContent replaces the log content and resets scroll.
func (lv *LogViewer) SetContent(content string) {
	lv.Content = content
	lv.rebuildLines()
	// Scroll to bottom by default.
	lv.scrollToBottom()
}

// SetSize updates available viewport dimensions.
func (lv *LogViewer) SetSize(width, height int) {
	lv.width = width
	lv.height = height
}

// IsEmpty reports whether the viewer has no content.
func (lv LogViewer) IsEmpty() bool {
	return lv.Content == ""
}

// IsActive reports whether a detail viewer session is open (title set).
func (lv LogViewer) IsActive() bool {
	return lv.Title != ""
}

// Append adds content while following logs and keeps the viewport at the bottom.
func (lv *LogViewer) Append(chunk string) {
	if chunk == "" {
		return
	}
	lv.Content += chunk
	lv.rebuildLines()
	lv.scrollToBottom()
}

// Reset clears all state.
func (lv *LogViewer) Reset() {
	lv.Title = ""
	lv.Content = ""
	lv.lines = nil
	lv.offset = 0
	lv.levelFilter = "ALL"
}

func (lv LogViewer) visibleRows() int {
	return max(1, lv.height-6)
}

func (lv *LogViewer) scrollToBottom() {
	vis := lv.visibleRows()
	if len(lv.lines) > vis {
		lv.offset = len(lv.lines) - vis
	} else {
		lv.offset = 0
	}
}

// Update handles key events for scrolling and copying.
func (lv LogViewer) Update(msg tea.Msg) (LogViewer, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		lv.width = msg.Width
		lv.height = msg.Height
		return lv, nil

	case tea.KeyMsg:
		return lv.handleKey(msg)
	}
	return lv, nil
}

func (lv LogViewer) handleKey(msg tea.KeyMsg) (LogViewer, tea.Cmd) {
	vis := lv.visibleRows()
	maxOffset := max(0, len(lv.lines)-vis)

	switch msg.String() {
	case "up", "k":
		if lv.offset > 0 {
			lv.offset--
		}
	case "down", "j":
		if lv.offset < maxOffset {
			lv.offset++
		}
	case "pgup":
		lv.offset = max(0, lv.offset-vis)
	case "pgdown":
		lv.offset = min(maxOffset, lv.offset+vis)
	case "home", "g":
		lv.offset = 0
	case "end", "G":
		lv.offset = maxOffset
	case "c":
		if lv.Content != "" {
			_ = clipboard.Copy(lv.Content)
			return lv, func() tea.Msg { return LogViewerCopy{Text: lv.Content} }
		}
	}

	return lv, nil
}

// View renders the log content.
func (lv LogViewer) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  " + lv.Title))
	sb.WriteString("\n\n")

	if lv.IsEmpty() {
		sb.WriteString(styles.MutedText.Render("  Loading logs..."))
		sb.WriteString("\n")
		return sb.String()
	}

	vis := lv.visibleRows()
	end := min(lv.offset+vis, len(lv.lines))

	for _, line := range lv.lines[lv.offset:end] {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Scroll position indicator.
	if len(lv.lines) > vis {
		pct := 0
		if len(lv.lines)-vis > 0 {
			pct = lv.offset * 100 / (len(lv.lines) - vis)
		}
		sb.WriteString(styles.MutedText.Render(
			fmt.Sprintf("\n  ─── %d lines (%d%%) ───  ↑/↓:scroll  PgUp/PgDn  g/G:top/bottom  c:copy",
				len(lv.lines), pct)))
	}

	return sb.String()
}
