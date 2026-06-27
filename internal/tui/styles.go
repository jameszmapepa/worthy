package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Dracula-leaning palette.
var (
	colorGreen      = lipgloss.Color("#50fa7b") // healthy >=70
	colorAmber      = lipgloss.Color("#f1fa8c") // mid 40-69
	colorRed        = lipgloss.Color("#ff5555") // poor <40
	colorFg         = lipgloss.Color("#f8f8f2") // foreground
	colorMuted      = lipgloss.Color("#6272a4") // comments/labels
	colorAccent     = lipgloss.Color("#bd93f9") // headers/accents
	colorBackground = lipgloss.Color("#282a36") // header bar background
	colorTrackEmpty = lipgloss.Color("#44475a") // unfilled bar track
	colorBorder     = lipgloss.Color("#44475a") // panel borders
)

// Per-category hues (Dracula) used to color radar axes and legends.
var (
	colorCatActivity  = lipgloss.Color("#8be9fd") // cyan
	colorCatCommunity = lipgloss.Color("#ffb86c") // orange
	colorCatSecurity  = lipgloss.Color("#ff79c6") // pink
)

// titledPanel wraps body in a rounded border with a small title label sitting
// on the top edge. Used to compose the multi-panel dashboard views.
func titledPanel(title, body string, border color.Color) string {
	titled := titleStyle.Render(title) + "\n" + body
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Render(titled)
}

// categoryColor maps a category key to its display hue.
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

// gradientSteps is the resolution of the score gradient: enough stops for a
// smooth red -> amber -> green sweep across a 0..100 bar.
const gradientSteps = 24

// scoreGradient is a precomputed red -> amber -> green color ramp. Bars index
// into it by score so the fill is a smooth gradient rather than a flat band.
var scoreGradient = lipgloss.Blend1D(gradientSteps, colorRed, colorAmber, colorGreen)

// gradientIndex maps a 0..100 value to an index into an n-color ramp, clamped
// to [0, n-1].
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
	i := int(value / 100 * float64(n-1))
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	return i
}

// gradientColor returns the score gradient color for a 0..100 value.
func gradientColor(value float64) color.Color {
	return scoreGradient[gradientIndex(value, len(scoreGradient))]
}

// Gate severity glyphs.
const (
	glyphInfo     = "ℹ"
	glyphWarn     = "⚠"
	glyphCritical = "✖"
)

// barFilled and barEmpty are the runes used for hand-rendered horizontal bars.
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

// questionCardStyle frames each question-answer headline card with a thick
// border so the two core answers are visually dominant above the detail panels.
var questionCardStyle = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(colorAccent).
	Padding(0, 2)

// severityStyle returns the glyph and color for a gate severity.
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

// stripANSI removes ANSI escape sequences, leaving the visible runes. Used for
// width math and in tests.
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
