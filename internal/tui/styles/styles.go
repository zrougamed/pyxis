// Package styles defines the shared visual theme for the TUI.
package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	Primary   = lipgloss.Color("#7C3AED") // Violet
	Secondary = lipgloss.Color("#06B6D4") // Cyan
	Success   = lipgloss.Color("#10B981") // Green
	Warning   = lipgloss.Color("#F59E0B") // Amber
	Error     = lipgloss.Color("#EF4444") // Red
	Muted     = lipgloss.Color("#6B7280") // Gray
	BgDark    = lipgloss.Color("#1F2937") // Dark gray
	White     = lipgloss.Color("#F9FAFB")

	// Base styles
	Title = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		Padding(0, 1)

	Subtitle = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true)

	StatusBar = lipgloss.NewStyle().
			Background(BgDark).
			Foreground(White).
			Padding(0, 1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	SelectedItem = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	NormalItem = lipgloss.NewStyle().
			Foreground(White)

	ErrorText = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	SuccessText = lipgloss.NewStyle().
			Foreground(Success)

	WarningText = lipgloss.NewStyle().
			Foreground(Warning)

	MutedText = lipgloss.NewStyle().
			Foreground(Muted)

	// Component styles
	InputField = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(0, 1)

	Card = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Muted).
		Padding(1, 2)

	CopiedBadge = lipgloss.NewStyle().
			Background(Success).
			Foreground(White).
			Padding(0, 1).
			Bold(true)
)

// PhaseStyle returns a styled string based on pod phase.
func PhaseStyle(phase string) lipgloss.Style {
	switch phase {
	case "Running":
		return lipgloss.NewStyle().Foreground(Success)
	case "Pending":
		return lipgloss.NewStyle().Foreground(Warning)
	case "Failed":
		return lipgloss.NewStyle().Foreground(Error)
	case "Succeeded":
		return lipgloss.NewStyle().Foreground(Secondary)
	default:
		return lipgloss.NewStyle().Foreground(Muted)
	}
}

// FormatAge formats a duration into a human-readable string.
func FormatAge(d interface{ Hours() float64 }) string {
	hours := d.Hours()
	switch {
	case hours < 1:
		return "<1h"
	case hours < 24:
		return fmt.Sprintf("%dh", int(hours))
	case hours < 24*30:
		return fmt.Sprintf("%dd", int(hours/24))
	default:
		return fmt.Sprintf("%dM", int(hours/(24*30)))
	}
}
