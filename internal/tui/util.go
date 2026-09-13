package tui

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

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

func clampWidth(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

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

// renderBar draws a gradient bar of width cells filled to value percent.
// Runs of equal colour share one escape sequence; the result is plain ANSI.
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
	filled := min(int(value/100*float64(width)+0.5), width)

	var b strings.Builder
	b.Grow(width*len(barFilled) + 32*(filled/2+2))

	prev := -1
	for i := range filled {
		cellValue := float64(i+1) / float64(width) * 100
		ci := gradientIndex(cellValue, len(scoreGradient))
		if ci != prev {
			b.WriteString(gradientSGR[ci])
			prev = ci
		}
		b.WriteString(barFilled)
	}
	if width-filled > 0 {
		b.WriteString(trackSGR)
		for range width - filled {
			b.WriteString(barEmpty)
		}
	}
	if width > 0 {
		b.WriteString(ansi.ResetStyle)
	}
	return b.String()
}

var sparklineRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

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
	for col := range width {
		i := col * n / width
		if i >= n {
			i = n - 1
		}
		v := series[i]

		var ri int
		if mx == mn {
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
