package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// palette is the full colour set for one background mode. Every style below
// is rebuilt from it by applyPalette, so a theme switch is one assignment.
type palette struct {
	green, amber, red                      color.Color
	fg, muted, accent                      color.Color
	trackEmpty, border                     color.Color
	catActivity, catCommunity, catSecurity color.Color
	star, fork, watcher                    color.Color
}

// darkPalette is Dracula.
var darkPalette = palette{
	green: lipgloss.Color("#50fa7b"), amber: lipgloss.Color("#f1fa8c"), red: lipgloss.Color("#ff5555"),
	fg: lipgloss.Color("#f8f8f2"), muted: lipgloss.Color("#6272a4"), accent: lipgloss.Color("#bd93f9"),
	trackEmpty: lipgloss.Color("#44475a"), border: lipgloss.Color("#44475a"),
	catActivity: lipgloss.Color("#8be9fd"), catCommunity: lipgloss.Color("#ffb86c"), catSecurity: lipgloss.Color("#ff79c6"),
	star: lipgloss.Color("#f1fa8c"), fork: lipgloss.Color("#bd93f9"), watcher: lipgloss.Color("#8be9fd"),
}

// lightPalette keeps the same hue roles at contrasts that read on a light
// background (Dracula's yellows and cyans wash out on white).
var lightPalette = palette{
	green: lipgloss.Color("#1f8a3b"), amber: lipgloss.Color("#b26a00"), red: lipgloss.Color("#c62828"),
	fg: lipgloss.Color("#282a36"), muted: lipgloss.Color("#6b6f85"), accent: lipgloss.Color("#6f42c1"),
	trackEmpty: lipgloss.Color("#d6d8e0"), border: lipgloss.Color("#b9bcc8"),
	catActivity: lipgloss.Color("#0e7490"), catCommunity: lipgloss.Color("#c2410c"), catSecurity: lipgloss.Color("#be185d"),
	star: lipgloss.Color("#b26a00"), fork: lipgloss.Color("#6f42c1"), watcher: lipgloss.Color("#0e7490"),
}

// colorBadgeInk is the text colour on filled gate badges; the badge fills are
// bright in both palettes, so the ink stays dark.
var colorBadgeInk = lipgloss.Color("#282a36")

var (
	colorGreen      color.Color
	colorAmber      color.Color
	colorRed        color.Color
	colorFg         color.Color
	colorMuted      color.Color
	colorAccent     color.Color
	colorTrackEmpty color.Color
	colorBorder     color.Color

	colorCatActivity  color.Color
	colorCatCommunity color.Color
	colorCatSecurity  color.Color

	colorStar    color.Color
	colorFork    color.Color
	colorWatcher color.Color
)

var (
	titleStyle lipgloss.Style
	labelStyle lipgloss.Style
	mutedStyle lipgloss.Style
	gradeStyle lipgloss.Style
	errStyle   lipgloss.Style

	questionCardStyle   lipgloss.Style
	panelStyle          lipgloss.Style
	heroStyle           lipgloss.Style
	selectedMarkerStyle lipgloss.Style
	selectedLabelStyle  lipgloss.Style
	detailStyle         lipgloss.Style
	headerPanelStyle    lipgloss.Style
	helpPanelStyle      lipgloss.Style
)

const gradientSteps = 24

var scoreGradient []color.Color

var currentPaletteDark = true

func init() { applyPalette(darkPalette, true) }

// applyPalette rebuilds every package-level colour and style from p. Bubble
// Tea calls Update and View on one goroutine, so the swap is race-free at
// runtime; tests that switch palettes must restore the dark one.
func applyPalette(p palette, dark bool) {
	currentPaletteDark = dark
	colorGreen, colorAmber, colorRed = p.green, p.amber, p.red
	colorFg, colorMuted, colorAccent = p.fg, p.muted, p.accent
	colorTrackEmpty, colorBorder = p.trackEmpty, p.border
	colorCatActivity, colorCatCommunity, colorCatSecurity = p.catActivity, p.catCommunity, p.catSecurity
	colorStar, colorFork, colorWatcher = p.star, p.fork, p.watcher

	titleStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	labelStyle = lipgloss.NewStyle().Foreground(colorFg)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	gradeStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	errStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)

	questionCardStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(colorAccent).
		Padding(0, 2)
	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)
	heroStyle = lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(colorAccent).
		Padding(0, 2)
	selectedMarkerStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedLabelStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	detailStyle = lipgloss.NewStyle().
		MarginLeft(2).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorAccent).
		PaddingLeft(1)
	headerPanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)
	helpPanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 2)

	scoreGradient = lipgloss.Blend1D(gradientSteps, colorRed, colorAmber, colorGreen)
}

// setTheme selects the palette for a dark or light terminal background.
func setTheme(dark bool) {
	if dark {
		applyPalette(darkPalette, true)
		return
	}
	applyPalette(lightPalette, false)
}

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
