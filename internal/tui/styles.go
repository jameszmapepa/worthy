package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	colorGreen      = lipgloss.Color("#50fa7b")
	colorAmber      = lipgloss.Color("#f1fa8c")
	colorRed        = lipgloss.Color("#ff5555")
	colorFg         = lipgloss.Color("#f8f8f2")
	colorMuted      = lipgloss.Color("#6272a4")
	colorAccent     = lipgloss.Color("#bd93f9")
	colorBackground = lipgloss.Color("#282a36")
	colorTrackEmpty = lipgloss.Color("#44475a")
	colorBorder     = lipgloss.Color("#44475a")
)

var (
	colorCatActivity  = lipgloss.Color("#8be9fd")
	colorCatCommunity = lipgloss.Color("#ffb86c")
	colorCatSecurity  = lipgloss.Color("#ff79c6")
)

var (
	colorStar    = lipgloss.Color("#f1fa8c")
	colorFork    = lipgloss.Color("#bd93f9")
	colorWatcher = lipgloss.Color("#8be9fd")
)

func titledPanel(title, body string, border color.Color) string {
	titled := titleStyle.Render(title) + "\n" + body
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Render(titled)
}

func categoryColor(key string) color.Color {
	switch key {
	case "activity":
		return colorCatActivity
	case "community":
		return colorCatCommunity
	case "security":
		return colorCatSecurity
	default:
		return colorFg
	}
}

const gradientSteps = 24

var scoreGradient = lipgloss.Blend1D(gradientSteps, colorRed, colorAmber, colorGreen)

func gradientIndex(value float64, n int) int {
	if n <= 1 {
		return 0
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	i := min(max(int(value/100*float64(n-1)), 0), n-1)
	return i
}

const (
	glyphInfo     = "ℹ"
	glyphWarn     = "⚠"
	glyphCritical = "✖"
)

const (
	barFilled = "█"
	barEmpty  = "░"
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(colorFg)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	gradeStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	errStyle   = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
)

var questionCardStyle = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(colorAccent).
	Padding(0, 2)

func severityGlyph(severity string) (string, color.Color) {
	switch severity {
	case "critical":
		return glyphCritical, colorRed
	case "warn":
		return glyphWarn, colorAmber
	default:
		return glyphInfo, colorMuted
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
