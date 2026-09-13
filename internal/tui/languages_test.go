package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/score"
)

func sampleShares() []score.LanguageShare {
	return score.LanguageShares(map[string]int{
		"Rust": 5260, "HTML": 3380, "TypeScript": 710, "Svelte": 470, "JavaScript": 60, "Just": 40, "Nix": 30, "Shell": 50,
	})
}

func TestLanguageBarFillsExactWidth(t *testing.T) {
	for _, w := range []int{20, 40, 96} {
		bar := languageBar(sampleShares(), w)
		if got := lipgloss.Width(bar); got != w {
			t.Errorf("bar width at %d = %d", w, got)
		}
	}
	if languageBar(nil, 40) != "" {
		t.Error("no shares should render no bar")
	}
}

func TestLanguageLegendTopSixPlusOther(t *testing.T) {
	out := strings.ReplaceAll(stripANSI(languageLegend(sampleShares(), 120)), nbsp, " ")
	for _, want := range []string{"Rust 52.6%", "HTML 33.8%", "TypeScript 7.1%", "Other"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "Nix") {
		t.Errorf("seventh language should fold into Other: %s", out)
	}
}

func TestHeaderShowsLanguagesWhenLoaded(t *testing.T) {
	raw := fixedRaw()
	raw.Languages = sampleShares()
	out := strings.ReplaceAll(stripANSI(renderHeaderPanel("o", "r", raw, true, false, github.RateInfo{}, 100, "B", false)), nbsp, " ")
	if !strings.Contains(out, "Rust 52.6%") {
		t.Errorf("header should carry the language legend:\n%s", out)
	}
	for i, line := range strings.Split(renderHeaderPanel("o", "r", raw, true, false, github.RateInfo{}, 60, "B", false), "\n") {
		if lw := lipgloss.Width(line); lw > 60 {
			t.Errorf("header line %d is %d wide at 60 cols", i, lw)
		}
	}
}

func TestLanguageBadgeIsFilled(t *testing.T) {
	glyph := languageBadge("Go", false)
	if !strings.Contains(glyph, "48;2;0;173;216") || !strings.Contains(stripANSI(glyph), " Go ") {
		t.Errorf("badge should paint the Go colour behind the name: %q", glyph)
	}
	ascii := stripANSI(languageBadge("Go", true))
	if strings.Contains(ascii, "") || !strings.Contains(ascii, "Go") {
		t.Errorf("ascii badge = %q", ascii)
	}
}

func TestLegendEntriesWrapAsUnits(t *testing.T) {
	out := stripANSI(languageLegend(sampleShares(), 40))
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), "●") || strings.HasPrefix(strings.TrimSpace(line), "%") {
			t.Errorf("legend split an entry across lines:\n%s", out)
		}
	}
}

func TestLanguageBarTerminatesWhenLanguagesOutnumberCells(t *testing.T) {
	many := make(map[string]int, 40)
	for i := range 40 {
		many[fmt.Sprintf("Lang%02d", i)] = 25
	}
	for _, w := range []int{20, 24, 30} {
		if got := lipgloss.Width(languageBar(score.LanguageShares(many), w)); got != w {
			t.Errorf("40 languages at width %d rendered %d cells", w, got)
		}
	}
	skewed := map[string]int{"Big": 7300}
	for i := range 45 {
		skewed[fmt.Sprintf("Tiny%02d", i)] = 60
	}
	if got := lipgloss.Width(languageBar(score.LanguageShares(skewed), 44)); got != 44 {
		t.Errorf("skewed set at width 44 rendered %d cells", got)
	}
}

func TestBadgeInkFollowsBackgroundLuminance(t *testing.T) {
	if !strings.Contains(languageBadge("PowerShell", true), "38;2;248;248;242") {
		t.Error("dark background should get light ink")
	}
	if !strings.Contains(languageBadge("JavaScript", true), "38;2;40;42;54") {
		t.Error("light background should get dark ink")
	}
	if !strings.Contains(languageBadge("PHP", true), "38;2;248;248;242") {
		t.Error("mid-dark PHP blue should get light ink under gamma-corrected luminance")
	}
}

func TestUnknownLanguagesGetDistinctColours(t *testing.T) {
	if languageColor("Foo") == languageColor("Barbaz") && languageColor("Foo") == languageColor("Quux1") {
		t.Error("fallback colours should vary by name")
	}
}
