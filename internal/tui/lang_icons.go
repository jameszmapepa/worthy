package tui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

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
	"c#":               lipgloss.Color("#178600"),
	"csharp":           lipgloss.Color("#178600"),
	"dart":             lipgloss.Color("#00b4ab"),
	"elixir":           lipgloss.Color("#6e4a7e"),
	"scala":            lipgloss.Color("#c22d40"),
	"objective-c":      lipgloss.Color("#438eff"),
	"perl":             lipgloss.Color("#0298c3"),
	"r":                lipgloss.Color("#198ce7"),
	"haskell":          lipgloss.Color("#5e5086"),
	"clojure":          lipgloss.Color("#db5855"),
	"svelte":           lipgloss.Color("#ff3e00"),
	"just":             lipgloss.Color("#384d54"),
	"dockerfile":       lipgloss.Color("#384d54"),
	"makefile":         lipgloss.Color("#427819"),
	"zig":              lipgloss.Color("#ec915c"),
	"nix":              lipgloss.Color("#7e7eff"),
	"elm":              lipgloss.Color("#60b5cc"),
	"ocaml":            lipgloss.Color("#ef7a08"),
	"scss":             lipgloss.Color("#c6538c"),
	"astro":            lipgloss.Color("#ff5a03"),
	"mdx":              lipgloss.Color("#fcb32c"),
	"jupyter notebook": lipgloss.Color("#da5b0b"),
	"powershell":       lipgloss.Color("#012456"),
	"batchfile":        lipgloss.Color("#c1f12e"),
	"cmake":            lipgloss.Color("#da3434"),
	"hcl":              lipgloss.Color("#844fba"),
	"erlang":           lipgloss.Color("#b83998"),
	"julia":            lipgloss.Color("#a270ba"),
	"groovy":           lipgloss.Color("#4298b8"),
	"assembly":         lipgloss.Color("#6e4c13"),
	"emacs lisp":       lipgloss.Color("#c065db"),
	"vim script":       lipgloss.Color("#199f4b"),
	"tex":              lipgloss.Color("#3d6117"),
	"gherkin":          lipgloss.Color("#5b2063"),
	"handlebars":       lipgloss.Color("#f7931e"),
	"blade":            lipgloss.Color("#f7523f"),
	"twig":             lipgloss.Color("#c1d026"),
	"less":             lipgloss.Color("#1d365d"),
	"wgsl":             lipgloss.Color("#1a5e9a"),
	"glsl":             lipgloss.Color("#5686a5"),
	"cuda":             lipgloss.Color("#3a4e3a"),
	"fortran":          lipgloss.Color("#4d41b1"),
	"solidity":         lipgloss.Color("#aa6746"),
	"gleam":            lipgloss.Color("#ffaff3"),
	"mojo":             lipgloss.Color("#ff4c1f"),
	"typst":            lipgloss.Color("#239dad"),
}

var fallbackLanguageColors = []color.Color{
	lipgloss.Color("#8be9fd"), lipgloss.Color("#ffb86c"), lipgloss.Color("#ff79c6"),
	lipgloss.Color("#bd93f9"), lipgloss.Color("#50fa7b"), lipgloss.Color("#f1fa8c"),
}

func languageColor(lang string) color.Color {
	key := strings.ToLower(strings.TrimSpace(lang))
	if ic, ok := languageIcons[key]; ok {
		return ic.color
	}
	if c, ok := languageColors[key]; ok {
		return c
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return fallbackLanguageColors[int(h.Sum32()%uint32(len(fallbackLanguageColors)))]
}

func inkFor(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA()
	luminance := 0.2126*linearChannel(r) + 0.7152*linearChannel(g) + 0.0722*linearChannel(b)
	if luminance < 0.18 {
		return lipgloss.Color("#f8f8f2")
	}
	return colorBadgeInk
}

func linearChannel(v uint32) float64 {
	c := float64(v) / 65535
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func languageBadge(lang string, ascii bool) string {
	key := strings.ToLower(strings.TrimSpace(lang))
	text := lang
	if ic, ok := languageIcons[key]; ok {
		text = ic.glyph + " " + lang
		if ascii {
			text = ic.tag
		}
	}
	bg := languageColor(lang)
	return lipgloss.NewStyle().
		Background(bg).
		Foreground(inkFor(bg)).
		Bold(true).
		Padding(0, 1).
		Render(text)
}

const (
	legendMaxEntries = 6
	nbsp             = "\u00a0"
)

func languageBar(shares []score.LanguageShare, width int) string {
	if width < 4 || len(shares) == 0 {
		return ""
	}
	cells := allocateCells(shares, width)
	var b strings.Builder
	for i, sh := range shares {
		if cells[i] == 0 {
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(languageColor(sh.Name)).Render(strings.Repeat(barFilled, cells[i])))
	}
	return b.String()
}

func allocateCells(shares []score.LanguageShare, width int) []int {
	cells := make([]int, len(shares))
	used := 0
	for i, sh := range shares {
		cells[i] = int(sh.Percent/100*float64(width) + 0.5)
		if cells[i] == 0 && sh.Percent >= 0.5 {
			cells[i] = 1
		}
		used += cells[i]
	}
	for used > width {
		shrink := -1
		for i, c := range cells {
			if c > 1 && (shrink < 0 || c > cells[shrink]) {
				shrink = i
			}
		}
		if shrink < 0 {
			for i := len(cells) - 1; i >= 0; i-- {
				if cells[i] > 0 {
					shrink = i
					break
				}
			}
		}
		cells[shrink]--
		used--
	}
	if used < width {
		cells[0] += width - used
	}
	return cells
}

func languageLegend(shares []score.LanguageShare, width int) string {
	entries := make([]string, 0, legendMaxEntries+1)
	other := 0.0
	for i, sh := range shares {
		if i >= legendMaxEntries {
			other += sh.Percent
			continue
		}
		dot := lipgloss.NewStyle().Foreground(languageColor(sh.Name)).Render("●")
		entries = append(entries, dot+nbsp+labelStyle.Render(strings.ReplaceAll(sh.Name, " ", nbsp))+nbsp+mutedStyle.Render(fmt.Sprintf("%.1f%%", sh.Percent)))
	}
	if other > 0 {
		entries = append(entries, mutedStyle.Render("●")+nbsp+labelStyle.Render("Other")+nbsp+mutedStyle.Render(fmt.Sprintf("%.1f%%", other)))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(entries, "   "))
}
