package components

import (
	"strings"
	"testing"
)

func TestRenderBarUnknown(t *testing.T) {
	bar := renderBar(nil, 6)
	if !strings.Contains(bar, "░") {
		t.Fatalf("expected empty bar for unknown percent, got %q", bar)
	}
}

func TestRenderBarFilled(t *testing.T) {
	pct := 50.0
	bar := renderBar(&pct, 10)
	if !strings.Contains(bar, "█") {
		t.Fatalf("expected filled cells, got %q", bar)
	}
}

func TestRenderMetricsInlineEmpty(t *testing.T) {
	if got := RenderMetricsInline("", "", nil, nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestRenderMetricsInlineWithValues(t *testing.T) {
	cpu := 40.0
	mem := 80.0
	got := RenderMetricsInline("125m", "256Mi", &cpu, &mem)
	if !strings.Contains(got, "125m") || !strings.Contains(got, "256Mi") {
		t.Fatalf("expected labels in output, got %q", got)
	}
	if !strings.Contains(got, "CPU") || !strings.Contains(got, "MEM") {
		t.Fatalf("expected CPU/MEM labels, got %q", got)
	}
}

func TestGaugeView(t *testing.T) {
	pct := 25.0
	g := Gauge{Label: "CPU", Value: "250m", Percent: &pct, Width: 8}
	out := g.View()
	if !strings.Contains(out, "CPU") || !strings.Contains(out, "250m") {
		t.Fatalf("unexpected gauge output: %q", out)
	}
}

func TestRenderMetricsStripUnavailable(t *testing.T) {
	out := RenderMetricsStrip("", "", nil, nil, 80)
	if !strings.Contains(out, "metrics-server") {
		t.Fatalf("expected unavailable message, got %q", out)
	}
}
