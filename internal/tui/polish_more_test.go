package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/worthy/internal/score"
)

func TestLicenseLabelNoLicense(t *testing.T) {
	for _, spdx := range []string{"", "NOASSERTION"} {
		if got := licenseLabel(spdx); got != "no license" {
			t.Errorf("licenseLabel(%q) = %q, want %q", spdx, got, "no license")
		}
	}
	if got := licenseLabel("MIT"); got != "MIT" {
		t.Errorf("licenseLabel(MIT) = %q", got)
	}
}

func TestMetaRowWithoutLanguageOrLicense(t *testing.T) {
	raw := score.RawMetrics{Stars: 10, Forks: 2, Watchers: 3, RepoAgeDays: 400}
	out := metaRow(raw, false)
	if !strings.Contains(out, "no license") {
		t.Errorf("meta row should show 'no license' when none: %s", out)
	}
	if !strings.Contains(out, "1.1y old") {
		t.Errorf("meta row should show age: %s", out)
	}
}

func TestMetaRowWithLanguage(t *testing.T) {
	raw := score.RawMetrics{Language: "Go", LicenseSPDX: "MIT", Stars: 5000}
	out := metaRow(raw, false)
	if !strings.Contains(out, languageIcons["go"].glyph) {
		t.Errorf("meta row missing Go devicon glyph: %q", out)
	}
	if !strings.Contains(out, "5.0k") {
		t.Errorf("meta row missing stars: %q", out)
	}

	out2 := metaRow(score.RawMetrics{Language: "Brainfuck", Stars: 10}, false)
	if !strings.Contains(out2, "Brainfuck") {
		t.Errorf("unmapped language should keep its name: %q", out2)
	}
}

func TestLanguageBadge(t *testing.T) {
	if got := languageBadge("TypeScript", false); !strings.Contains(got, languageIcons["typescript"].glyph) {
		t.Errorf("mapped language should render its glyph: %q", got)
	}
	if strings.Contains(languageBadge("Go", false), "Go") {
		t.Errorf("mapped language should drop the name in favor of the glyph")
	}

	asciiTS := languageBadge("TypeScript", true)
	if !strings.Contains(asciiTS, languageIcons["typescript"].tag) {
		t.Errorf("ascii mode should render the tag %q: %q", languageIcons["typescript"].tag, asciiTS)
	}
	if strings.Contains(asciiTS, languageIcons["typescript"].glyph) {
		t.Errorf("ascii mode must not render the Nerd Font glyph: %q", asciiTS)
	}

	if got := languageBadge("COBOL", false); !strings.Contains(got, "COBOL") {
		t.Errorf("unmapped language should keep its name: %q", got)
	}

	if got := languageBadge("Dart", false); !strings.Contains(got, "Dart") {
		t.Errorf("languageColors language should keep its name: %q", got)
	}

	if _, inIcons := languageIcons["dart"]; inIcons {
		t.Fatal("test assumption broken: dart must NOT be in languageIcons")
	}
	if _, inColors := languageColors["dart"]; !inColors {
		t.Fatal("test assumption broken: dart must be in languageColors")
	}
}

func TestJoinEndsOverflow(t *testing.T) {
	out := joinEnds("aaaaa", "bbbbb", 6)
	if out != "aaaaa bbbbb" {
		t.Errorf("overflow joinEnds = %q, want single space", out)
	}

	out = joinEnds("a", "b", 10)
	if len(out) != 10 {
		t.Errorf("joinEnds width = %d, want 10 (%q)", len(out), out)
	}
}

func TestCategoryColorDefault(t *testing.T) {
	if categoryColor("unknown") != colorFg {
		t.Error("unknown category should fall back to foreground color")
	}
}

func TestVerdictAppearsInScorecardHero(t *testing.T) {
	r := realReport()
	out := renderScorecard(r, 100, -1, false)

	if r.Verdict == "" || !strings.Contains(out, "grade") {
		t.Errorf("scorecard hero should contain the verdict:\n%s", out)
	}
}
