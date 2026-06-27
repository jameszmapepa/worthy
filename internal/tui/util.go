package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// truncate shortens s to at most n runes, adding an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// clampWidth constrains v to [lo, hi].
func clampWidth(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// barColor maps a 0..100 score to its band color: green >=70, amber 40-69,
// red <40.
func barColor(value float64) color.Color {
	switch {
	case value >= 70:
		return colorGreen
	case value >= 40:
		return colorAmber
	default:
		return colorRed
	}
}

// renderBar renders a horizontal bar of the given total cell width for a
// 0..100 value. Each filled cell is colored by its own position along the
// red -> amber -> green gradient. Adjacent cells sharing the same gradient
// index are grouped into a single strings.Repeat + Render call (≤gradientSteps
// style allocs per bar instead of ≤width), keeping output visually identical
// to the naive per-cell version while cutting allocations by ~400/keypress on
// a full 24-stop gradient.
func renderBar(value float64, width int) string {
	if width < 1 {
		width = 1
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	filled := int(value/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}

	var b strings.Builder
	// Group adjacent filled cells that map to the same gradient color index
	// into a single Render call. With gradientSteps=24 the loop body executes
	// at most 24 times regardless of bar width.
	prev := -1
	runLen := 0
	for i := 0; i < filled; i++ {
		// Map this cell's position to a score so the gradient sweeps the full
		// bar, ending at the value's own color — identical to the old per-cell
		// mapping.
		cellValue := float64(i+1) / float64(width) * 100
		ci := gradientIndex(cellValue, len(scoreGradient))
		if ci == prev {
			runLen++
		} else {
			if prev >= 0 {
				b.WriteString(
					lipgloss.NewStyle().Foreground(scoreGradient[prev]).
						Render(strings.Repeat(barFilled, runLen)),
				)
			}
			prev = ci
			runLen = 1
		}
	}
	if prev >= 0 {
		b.WriteString(
			lipgloss.NewStyle().Foreground(scoreGradient[prev]).
				Render(strings.Repeat(barFilled, runLen)),
		)
	}

	track := lipgloss.NewStyle().Foreground(colorTrackEmpty).
		Render(strings.Repeat(barEmpty, width-filled))
	return b.String() + track
}

// sparklineRunes are the eight Unicode block elements, ordered lowest to
// highest, used by sparklineBlock to represent relative commit volumes.
var sparklineRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// sparklineBlock converts an integer series to a width-rune Unicode sparkline.
// Values are min/max normalized so the full block-rune range is always used;
// when all values are equal (flat data), the mid block '▄' is used.
// If len(series) > width the series is subsampled (nearest-index); if shorter
// it is stretched. This is the pure transformation; renderSparkline wraps it
// with the empty-data check and muted styling.
func sparklineBlock(series []int, width int) string {
	n := len(series)
	if n == 0 || width == 0 {
		return ""
	}

	mn, mx := series[0], series[0]
	for _, v := range series {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}

	nr := len(sparklineRunes)
	var b strings.Builder
	for col := 0; col < width; col++ {
		// Map terminal column to nearest data index (nearest-index subsample /
		// stretch — adequate for a 1-row sparkline where absolute fidelity is
		// less important than visual trend direction).
		i := col * n / width
		if i >= n {
			i = n - 1
		}
		v := series[i]

		var ri int
		if mx == mn {
			// Flat data: use the mid block rather than the lowest rune so a
			// uniform series looks neutral, not failed.
			ri = nr / 2
		} else {
			ri = int(float64(v-mn)/float64(mx-mn)*float64(nr-1) + 0.5)
			if ri >= nr {
				ri = nr - 1
			}
		}
		b.WriteRune(sparklineRunes[ri])
	}
	return b.String()
}
