package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// StatusStrip is a compact Lens-style status row for the active selection.
type StatusStrip struct {
	Kind      string
	Name      string
	Namespace string
	Status    string
	Extra     string
	SparkCPU  string
	SparkMem  string
}

// View renders the strip.
func (s StatusStrip) View(width int) string {
	if s.Name == "" {
		return ""
	}
	title := s.Kind
	if title == "" {
		title = "Resource"
	}
	id := s.Name
	if s.Namespace != "" {
		id = s.Namespace + "/" + s.Name
	}
	left := styles.Subtitle.Render(fmt.Sprintf(" %s ", title)) + " " + styles.NormalItem.Render(id)
	if s.Status != "" {
		left += "  " + styles.PhaseStyle(s.Status).Render(s.Status)
	}
	if s.Extra != "" {
		left += "  " + styles.MutedText.Render(s.Extra)
	}

	rightParts := make([]string, 0, 2)
	if s.SparkCPU != "" {
		rightParts = append(rightParts, styles.MutedText.Render("CPU ")+s.SparkCPU)
	}
	if s.SparkMem != "" {
		rightParts = append(rightParts, styles.MutedText.Render("MEM ")+s.SparkMem)
	}
	right := strings.Join(rightParts, "  ")

	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	line := left + strings.Repeat(" ", gap) + right
	return styles.StatusBar.Render(line)
}
