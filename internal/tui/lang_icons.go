package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ceiling: curated set; unmapped languages fall back to a colored dot + name.
type langIcon struct {
	glyph string
	tag   string
	color color.Color
}

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
