package components

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildSparkline_Empty(t *testing.T) {
	got := BuildSparkline(nil, 5)
	if got != "     " {
		t.Errorf("expected 5 spaces, got %q", got)
	}
	if BuildSparkline([]float64{1}, 0) != "" {
		t.Error("expected empty for width 0")
	}
}

func TestBuildSparkline_Width(t *testing.T) {
	got := BuildSparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7}, 8)
	if utf8.RuneCountInString(got) != 8 {
		t.Fatalf("expected 8 runes, got %d (%q)", utf8.RuneCountInString(got), got)
	}
	if !strings.ContainsRune(got, '▁') || !strings.ContainsRune(got, '█') {
		t.Errorf("expected min/max blocks in %q", got)
	}
}

func TestBuildSparkline_TruncatesToRecent(t *testing.T) {
	got := BuildSparkline([]float64{0, 0, 0, 7}, 2)
	if utf8.RuneCountInString(got) != 2 {
		t.Fatalf("expected 2 runes, got %q", got)
	}
}
