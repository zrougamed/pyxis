package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/zrougamed/pyxis/internal/tui/components"
	"github.com/zrougamed/pyxis/internal/tui/styles"
)

// View renders the entire UI.
func (m Model) View() string {
	var sb strings.Builder

	// Header.
	sb.WriteString(m.renderHeader())
	sb.WriteByte('\n')

	// Error.
	if m.err != nil {
		sb.WriteString(styles.ErrorText.Render(fmt.Sprintf("  Error: %v", m.err)))
		sb.WriteByte('\n')
		sb.WriteString(styles.HelpStyle.Render("  Press Esc to go back"))
		sb.WriteByte('\n')
		return sb.String()
	}

	// Confirm / prompt overlays.
	if m.confirmAction != confirmNone {
		sb.WriteString(m.renderConfirm())
		sb.WriteByte('\n')
		sb.WriteString(m.renderFooter())
		return sb.String()
	}
	if m.promptMode != "" || m.view == ViewPrompt {
		sb.WriteString(m.renderPrompt())
		sb.WriteByte('\n')
		sb.WriteString(m.renderFooter())
		return sb.String()
	}

	// Main content — delegate to components or render menu.
	switch {
	case m.view == ViewMenu:
		sb.WriteString(m.renderMenu())

	case m.view == ViewOverview:
		sb.WriteString(m.renderOverview())

	case m.view == ViewNamespaceExplore:
		sb.WriteString(m.renderNamespaceExplore())

	case m.view == ViewPortForwards:
		sb.WriteString(m.renderPortForwards())

	case m.logViewer.IsActive():
		sb.WriteString(m.logViewer.View())

	default:
		if strip := m.renderStatusStrip(); strip != "" {
			sb.WriteString(strip)
			sb.WriteByte('\n')
		}
		if strip := m.renderSelectedMetrics(); strip != "" {
			sb.WriteString(strip)
			sb.WriteByte('\n')
		}
		if m.view == ViewPodImages || m.view == ViewPodList {
			m.list.Footer = fmt.Sprintf("Filter: %s (press f to cycle)", FilterName(m.podFilter))
		}
		if m.watching {
			m.list.Footer = strings.TrimSpace(m.list.Footer + "  · watch on")
		}
		sb.WriteString(m.list.View())
	}

	// Footer.
	sb.WriteByte('\n')
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m Model) renderHeader() string {
	ctx := m.client.CurrentContext()
	ns := m.namespace
	if ns == "" {
		ns = "all"
	}

	left := styles.Title.Render("⎈ Pyxis")
	rightBits := fmt.Sprintf(" ctx:%s  ns:%s ", ctx, ns)
	if m.watching {
		rightBits = " watch● " + rightBits
	}
	right := styles.StatusBar.Render(rightBits)

	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

// renderStatusStrip shows a Lens-style selection strip with optional sparklines.
func (m Model) renderStatusStrip() string {
	sparkCPU := components.BuildSparkline(m.cpuHistory, 12)
	sparkMem := components.BuildSparkline(m.memHistory, 12)

	switch m.view {
	case ViewPodList, ViewPodLogs, ViewPodEnv:
		item := m.list.SelectedItem()
		if item == nil {
			return ""
		}
		idx, err := strconv.Atoi(item.ID)
		if err != nil || idx < 0 || idx >= len(m.pods) {
			return ""
		}
		pod := m.pods[idx]
		extra := fmt.Sprintf("Ready:%s Restarts:%d Node:%s", pod.Ready, pod.Restarts, pod.Node)
		return components.StatusStrip{
			Kind:      "Pod",
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Phase),
			Extra:     extra,
			SparkCPU:  sparkCPU,
			SparkMem:  sparkMem,
		}.View(m.width)

	case ViewNodes:
		item := m.list.SelectedItem()
		if item == nil {
			return ""
		}
		for _, r := range m.resources {
			if r.Name == item.ID || r.Name == item.Name {
				extra := ""
				if v, ok := r.Extra["version"]; ok {
					extra = v
				}
				return components.StatusStrip{
					Kind:     "Node",
					Name:     r.Name,
					Status:   r.Status,
					Extra:    extra,
					SparkCPU: sparkCPU,
					SparkMem: sparkMem,
				}.View(m.width)
			}
		}
	}
	return ""
}

// renderSelectedMetrics shows gauges for the highlighted list row.
func (m Model) renderSelectedMetrics() string {
	switch m.view {
	case ViewPodList, ViewPodLogs, ViewPodEnv:
		item := m.list.SelectedItem()
		if item == nil {
			return ""
		}
		idx, err := strconv.Atoi(item.ID)
		if err != nil || idx < 0 || idx >= len(m.pods) {
			return ""
		}
		pod := m.pods[idx]
		if pod.CPULabel == "" && pod.MemoryLabel == "" && pod.DiskLabel == "" && pod.NetworkLabel == "" {
			return ""
		}
		return components.RenderMetricsStrip4(
			pod.CPULabel, pod.MemoryLabel, pod.DiskLabel, pod.NetworkLabel,
			pod.CPUPercent, pod.MemoryPercent, pod.DiskPercent, pod.NetworkPercent,
			m.width,
		)

	case ViewNodes:
		item := m.list.SelectedItem()
		if item == nil {
			return ""
		}
		for _, r := range m.resources {
			if r.Name == item.ID || r.Name == item.Name {
				if r.CPULabel == "" && r.MemoryLabel == "" && r.DiskLabel == "" && r.NetworkLabel == "" {
					return ""
				}
				return components.RenderMetricsStrip4(
					r.CPULabel, r.MemoryLabel, r.DiskLabel, r.NetworkLabel,
					r.CPUPercent, r.MemoryPercent, r.DiskPercent, r.NetworkPercent,
					m.width,
				)
			}
		}
	}
	return ""
}

func (m Model) renderMenu() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("  What would you like to do?"))
	sb.WriteString("\n\n")

	for i, item := range mainMenu {
		cursor := "  "
		style := styles.NormalItem
		if i == m.menuCursor {
			cursor = styles.SelectedItem.Render("▸ ")
			style = styles.SelectedItem
		}

		title := style.Render(item.title)
		desc := styles.MutedText.Render(" — " + item.desc)
		sb.WriteString(fmt.Sprintf("%s%s%s\n", cursor, title, desc))
	}

	return sb.String()
}

func (m Model) renderFooter() string {
	var parts []string

	if m.copied && time.Since(m.copiedTime) < 2*time.Second {
		parts = append(parts, styles.CopiedBadge.Render(" ✓ Copied "))
	}

	if m.statusMsg != "" && time.Since(m.statusTime) < 5*time.Second {
		parts = append(parts, styles.SuccessText.Render("  "+m.statusMsg))
	}

	help := m.contextualHelp()
	parts = append(parts, styles.HelpStyle.Render("  "+help))

	return strings.Join(parts, "  ")
}

func (m Model) contextualHelp() string {
	if m.confirmAction != confirmNone {
		return "y:confirm  n/esc:cancel"
	}
	if m.promptMode != "" || m.view == ViewPrompt {
		return "enter:submit  esc:cancel"
	}
	if m.view == ViewOverview {
		watch := "w:watch"
		if m.watching {
			watch = "w:watch✓"
		}
		return fmt.Sprintf("r:refresh  %s  esc:back  q:menu", watch)
	}
	if m.view == ViewNamespaceExplore {
		return "↑/↓:navigate  enter:open  y:yaml  esc:back"
	}
	if m.view == ViewPortForwards {
		return "↑/↓:navigate  d:stop  esc:back  q:menu"
	}
	if m.logViewer.IsActive() {
		if m.view == ViewYAMLDetail {
			return "↑/↓:scroll  Y:lint  A:dry-run  a:apply  C/c:copy  esc:back  q:menu"
		}
		live := "l:live"
		if m.following {
			live = "l:live✓"
		}
		level := fmt.Sprintf("L:%s", m.logLevel)
		return fmt.Sprintf("↑/↓:scroll  PgUp/PgDn  g/G:top/bottom  c:copy  %s  %s  esc:back  q:menu", live, level)
	}

	watch := "w:watch"
	if m.watching {
		watch = "w:watch✓"
	}

	switch m.view {
	case ViewMenu:
		return "↑/↓:navigate  enter:select  q:quit"
	case ViewPodImages:
		return fmt.Sprintf("/:search  f:filter  enter:select  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	case ViewPodList:
		return fmt.Sprintf("/:search  f:filter  enter:inspect  y:yaml  d:delete  p:port-fwd  x:shell  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	case ViewPodLogs:
		return fmt.Sprintf("/:search  enter:logs  d:delete  y:yaml  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	case ViewPodEnv:
		return fmt.Sprintf("/:search  enter:env  C:copy-value  y:yaml  c:copy  r:refresh  esc:back  q:menu")
	case ViewContainerSelect:
		return "/:search  enter:select  esc:back  q:menu"
	case ViewDeployments, ViewStatefulSets:
		return fmt.Sprintf("/:search  enter:inspect  y:yaml  R:restart  +/-:scale  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	case ViewDaemonSets:
		return fmt.Sprintf("/:search  enter:inspect  y:yaml  R:restart  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	case ViewNamespaces:
		return "/:search  enter:explore  n:create  d:delete  y:yaml  c:copy  r:refresh  esc:back  q:menu"
	case ViewConfigMaps, ViewSecrets, ViewPVs, ViewPVCs, ViewServices, ViewIngresses, ViewJobs, ViewCronJobs, ViewHPAs, ViewNodes, ViewEvents, ViewCRDs, ViewHelm:
		return fmt.Sprintf("/:search  enter:select  y:yaml  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	default:
		return fmt.Sprintf("/:search  enter:select  y:yaml  c:copy  r:refresh  %s  esc:back  q:menu", watch)
	}
}
