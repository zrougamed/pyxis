package components

import "strings"

var sparklineBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// BuildSparkline renders a unicode block sparkline from values.
// Empty or non-positive width returns ""; nil/empty values return spaces of width.
func BuildSparkline(values []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat(" ", width)
	}

	// Sample or pad to exactly width columns (keep most recent when truncating).
	samples := resample(values, width)

	minV, maxV := samples[0], samples[0]
	for _, v := range samples[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	var b strings.Builder
	b.Grow(width * 3) // unicode blocks are multi-byte
	span := maxV - minV
	for _, v := range samples {
		idx := 0
		if span > 0 {
			idx = int((v - minV) / span * float64(len(sparklineBlocks)-1))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(sparklineBlocks) {
				idx = len(sparklineBlocks) - 1
			}
		}
		b.WriteRune(sparklineBlocks[idx])
	}
	return b.String()
}

func resample(values []float64, width int) []float64 {
	n := len(values)
	if n == width {
		out := make([]float64, width)
		copy(out, values)
		return out
	}
	if n > width {
		// Keep the most recent width samples.
		out := make([]float64, width)
		copy(out, values[n-width:])
		return out
	}
	// Pad on the left with the first value (or 0).
	out := make([]float64, width)
	pad := width - n
	fill := 0.0
	if n > 0 {
		fill = values[0]
	}
	for i := 0; i < pad; i++ {
		out[i] = fill
	}
	copy(out[pad:], values)
	return out
}
