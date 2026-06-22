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
// red -> amber -> green gradient, so the bar reads as a smooth sweep rather
// than a flat band.
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
	for i := 0; i < filled; i++ {
		// Map this cell's position (0..width) to a score so the gradient spans
		// the full bar, ending at the value's own color.
		cellValue := float64(i+1) / float64(width) * 100
		b.WriteString(lipgloss.NewStyle().Foreground(gradientColor(cellValue)).Render(barFilled))
	}
	track := lipgloss.NewStyle().Foreground(colorTrackEmpty).
		Render(strings.Repeat(barEmpty, width-filled))
	return b.String() + track
}
