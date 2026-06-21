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
)

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
	headerStyle = lipgloss.NewStyle().
			Foreground(colorFg).
			Background(colorBackground).
			Bold(true).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(colorFg)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	gradeStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	errStyle   = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
)

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

// renderHeader renders the top bar: repo identity and effective rate-limit mode.
func renderHeader(owner, repo string, authenticated bool, width int) string {
	mode := "60/hr unauthenticated"
	if authenticated {
		mode = "5,000/hr authenticated"
	}
	left := titleStyle.Render("repo-health") + "  " + owner + "/" + repo
	right := mutedStyle.Render(mode)
	bar := left + "   " + right
	return headerStyle.Width(maxInt(width, lipgloss.Width(bar))).Render(bar)
}

// renderBar renders a colored horizontal bar of the given total cell width for
// a 0..100 value. The fill color follows barColor.
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
	fill := lipgloss.NewStyle().Foreground(barColor(value)).
		Render(strings.Repeat(barFilled, filled))
	track := lipgloss.NewStyle().Foreground(colorTrackEmpty).
		Render(strings.Repeat(barEmpty, width-filled))
	return fill + track
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
