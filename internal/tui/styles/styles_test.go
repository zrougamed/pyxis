package styles

import (
	"testing"
	"time"
)

type mockDuration struct {
	hours float64
}

func (d mockDuration) Hours() float64 { return d.hours }

func TestFormatAge(t *testing.T) {
	tests := []struct {
		hours float64
		want  string
	}{
		{0.5, "<1h"},
		{3, "3h"},
		{25, "1d"},
		{48, "2d"},
		{24 * 45, "1M"},
	}

	for _, tt := range tests {
		got := FormatAge(mockDuration{tt.hours})
		if got != tt.want {
			t.Errorf("FormatAge(%f hours) = %q, want %q", tt.hours, got, tt.want)
		}
	}
}

func TestFormatAgeWithTimeDuration(t *testing.T) {
	d := 5 * time.Hour
	got := FormatAge(d)
	if got != "5h" {
		t.Errorf("FormatAge(5h) = %q, want '5h'", got)
	}
}

func TestPhaseStyle(t *testing.T) {
	// Just ensure these don't panic.
	phases := []string{"Running", "Pending", "Failed", "Succeeded", "Unknown"}
	for _, p := range phases {
		s := PhaseStyle(p)
		_ = s.Render(p) // Should not panic.
	}
}
