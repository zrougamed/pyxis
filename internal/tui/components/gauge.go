// Package components provides reusable Bubble Tea sub-models.
package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// Gauge is a labeled horizontal utilisation bar for CPU/memory metrics.
type Gauge struct {
	Label   string
	Value   string
	Percent *float64 // nil when utilisation baseline is unknown
	Width   int      // bar width in characters; defaults to 10
}

// View renders a single gauge line: "CPU [████░░░░░░] 45%  125m"
func (g Gauge) View() string {
	width := g.Width
	if width <= 0 {
		width = 10
	}

	bar := renderBar(g.Percent, width)
	pctText := "n/a"
	style := styles.MutedText
	if g.Percent != nil {
		pctText = fmt.Sprintf("%.0f%%", *g.Percent)
		style = gaugeStyle(*g.Percent)
	}

	label := styles.MutedText.Render(g.Label)
	value := g.Value
	if value == "" {
		value = "—"
	}

	return fmt.Sprintf("%s %s %s  %s",
		label,
		bar,
		style.Render(pctText),
		styles.NormalItem.Render(value),
	)
}

// RenderMetricsInline returns a compact "CPU:… MEM:…" suffix for list rows.
func RenderMetricsInline(cpuLabel, memLabel string, cpuPct, memPct *float64) string {
	if cpuLabel == "" && memLabel == "" {
		return ""
	}
	parts := make([]string, 0, 2)
	if cpuLabel != "" {
		parts = append(parts, formatInlineMetric("CPU", cpuLabel, cpuPct))
	}
	if memLabel != "" {
		parts = append(parts, formatInlineMetric("MEM", memLabel, memPct))
	}
	return "  " + strings.Join(parts, "  ")
}

// RenderMetricsStrip renders two gauges side by side for detail headers.
func RenderMetricsStrip(cpuLabel, memLabel string, cpuPct, memPct *float64, width int) string {
	return RenderMetricsStrip4(cpuLabel, memLabel, "", "", cpuPct, memPct, nil, nil, width)
}

// RenderMetricsStrip4 renders CPU/MEM/DISK/NET gauges.
func RenderMetricsStrip4(cpuLabel, memLabel, diskLabel, netLabel string, cpuPct, memPct, diskPct, netPct *float64, width int) string {
	if cpuLabel == "" && memLabel == "" && diskLabel == "" && netLabel == "" {
		return styles.MutedText.Render("  Metrics unavailable (is metrics-server installed?)")
	}
	parts := make([]string, 0, 4)
	if cpuLabel != "" || cpuPct != nil {
		parts = append(parts, Gauge{Label: "CPU", Value: cpuLabel, Percent: cpuPct, Width: 10}.View())
	}
	if memLabel != "" || memPct != nil {
		parts = append(parts, Gauge{Label: "MEM", Value: memLabel, Percent: memPct, Width: 10}.View())
	}
	if diskLabel != "" || diskPct != nil {
		parts = append(parts, Gauge{Label: "DISK", Value: diskLabel, Percent: diskPct, Width: 10}.View())
	}
	if netLabel != "" || netPct != nil {
		parts = append(parts, Gauge{Label: "NET", Value: netLabel, Percent: netPct, Width: 10}.View())
	}
	joined := "  " + strings.Join(parts, "    ")
	if width > 0 && lipgloss.Width(joined) > width {
		return "  " + strings.Join(parts, "\n  ")
	}
	return joined
}

// RenderMetricsInline4 returns compact CPU/MEM/DISK/NET suffixes for list rows.
func RenderMetricsInline4(cpuLabel, memLabel, diskLabel, netLabel string, cpuPct, memPct, diskPct, netPct *float64) string {
	parts := make([]string, 0, 4)
	if cpuLabel != "" {
		parts = append(parts, formatInlineMetric("CPU", cpuLabel, cpuPct))
	}
	if memLabel != "" {
		parts = append(parts, formatInlineMetric("MEM", memLabel, memPct))
	}
	if diskLabel != "" {
		parts = append(parts, formatInlineMetric("DISK", diskLabel, diskPct))
	}
	if netLabel != "" {
		parts = append(parts, formatInlineMetric("NET", netLabel, netPct))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

func formatInlineMetric(label, value string, pct *float64) string {
	bar := renderBar(pct, 6)
	styled := styles.MutedText.Render(label)
	if pct != nil {
		return fmt.Sprintf("%s:%s%s", styled, bar, gaugeStyle(*pct).Render(value))
	}
	return fmt.Sprintf("%s:%s%s", styled, bar, styles.MutedText.Render(value))
}

func renderBar(pct *float64, width int) string {
	filled := 0
	if pct != nil {
		filled = int((*pct / 100) * float64(width))
		if *pct > 0 && filled == 0 {
			filled = 1
		}
		if filled > width {
			filled = width
		}
	}
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	if pct == nil {
		return styles.MutedText.Render("[" + strings.Repeat("░", width) + "]")
	}
	return gaugeStyle(*pct).Render("[" + bar + "]")
}

func gaugeStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 90:
		return lipgloss.NewStyle().Foreground(styles.Error)
	case pct >= 75:
		return lipgloss.NewStyle().Foreground(styles.Warning)
	default:
		return lipgloss.NewStyle().Foreground(styles.Success)
	}
}
