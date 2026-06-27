package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// langIcon pairs a Nerd Font devicon glyph (and an ASCII-tag fallback) with the
// language's brand color.
//
// The glyphs are Nerd Font codepoints in the Unicode private-use area
// ("Seti-UI + Custom" and "Devicons" ranges). They render as the language mark
// only in a terminal using a patched Nerd Font; without one they show a
// missing-glyph box. A program cannot detect at runtime whether the font is
// Nerd-patched (terminals do not expose font info), so this is an explicit mode:
// when ascii mode is on (the --ascii flag or REPO_HEALTH_ASCII env) the short
// `tag` is rendered instead. The brand color renders in both modes.
//
// ceiling: this is a curated set of the most common GitHub languages. Unmapped
// languages fall back to a brand-colored dot plus the language name (see
// languageBadge), which never tofus and stays identifiable. Add entries here as
// needed; the glyph for an uncertain language should be left out rather than
// guessed, so the safe dot+name fallback is used instead.
type langIcon struct {
	glyph string
	tag   string
	color color.Color
}

// languageIcons maps a lower-cased GitHub "language" value to its devicon, an
// ASCII-tag fallback, and brand color (following GitHub Linguist's languages.yml).
var languageIcons = map[string]langIcon{
	"go":         {"", "Go", lipgloss.Color("#00add8")},
	"python":     {"", "Py", lipgloss.Color("#3572a5")},
	"rust":       {"", "Rs", lipgloss.Color("#dea584")},
	"typescript": {"", "TS", lipgloss.Color("#3178c6")},
	"javascript": {"", "JS", lipgloss.Color("#f1e05a")},
	"java":       {"", "Jv", lipgloss.Color("#b07219")},
	"ruby":       {"", "Rb", lipgloss.Color("#701516")},
	"php":        {"", "PHP", lipgloss.Color("#4f5d95")},
	"c":          {"", "C", lipgloss.Color("#555555")},
	"c++":        {"", "C++", lipgloss.Color("#f34b7d")},
	"html":       {"", "HTML", lipgloss.Color("#e34c26")},
	"css":        {"", "CSS", lipgloss.Color("#563d7c")},
	"shell":      {"", "Sh", lipgloss.Color("#89e051")},
	"swift":      {"", "Sw", lipgloss.Color("#f05138")},
	"kotlin":     {"", "Kt", lipgloss.Color("#a97bff")},
	"lua":        {"", "Lua", lipgloss.Color("#000080")},
	"vue":        {"", "Vue", lipgloss.Color("#41b883")},
	"markdown":   {"", "MD", lipgloss.Color("#083fa1")},
}

// languageColors covers languages we want a brand-colored dot for even without a
// confident glyph mapping — keeps the dot meaningful in the fallback path.
var languageColors = map[string]color.Color{
	"c#":          lipgloss.Color("#178600"),
	"csharp":      lipgloss.Color("#178600"),
	"dart":        lipgloss.Color("#00b4ab"),
	"elixir":      lipgloss.Color("#6e4a7e"),
	"scala":       lipgloss.Color("#c22d40"),
	"objective-c": lipgloss.Color("#438eff"),
	"perl":        lipgloss.Color("#0298c3"),
	"r":           lipgloss.Color("#198ce7"),
	"haskell":     lipgloss.Color("#5e5086"),
	"clojure":     lipgloss.Color("#db5855"),
}

// languageBadge renders the language as a compact icon. A mapped language shows
// its devicon glyph (or, in ascii mode, its short tag) in brand color — the name
// is dropped because the symbol is the label. An unmapped language shows a
// brand-colored dot followed by the name so it stays identifiable without a font
// dependency, in either mode.
func languageBadge(lang string, ascii bool) string {
	key := strings.ToLower(strings.TrimSpace(lang))
	if ic, ok := languageIcons[key]; ok {
		sym := ic.glyph
		if ascii {
			sym = ic.tag
		}
		return lipgloss.NewStyle().Foreground(ic.color).Render(sym)
	}
	dotColor := colorMuted
	if c, ok := languageColors[key]; ok {
		dotColor = c
	}
	dot := lipgloss.NewStyle().Foreground(dotColor).Render("●")
	return dot + " " + labelStyle.Render(lang)
}
