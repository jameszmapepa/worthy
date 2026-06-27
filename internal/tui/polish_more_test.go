package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/repo-health/internal/score"
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
	// A mapped language renders as its devicon glyph (the name is dropped — the
	// glyph is the label), alongside the star count.
	raw := score.RawMetrics{Language: "Go", LicenseSPDX: "MIT", Stars: 5000}
	out := metaRow(raw, false)
	if !strings.Contains(out, languageIcons["go"].glyph) {
		t.Errorf("meta row missing Go devicon glyph: %q", out)
	}
	if !strings.Contains(out, "5.0k") {
		t.Errorf("meta row missing stars: %q", out)
	}

	// An unmapped language has no confident glyph, so it falls back to a dot plus
	// the language name and stays legible without a Nerd Font.
	out2 := metaRow(score.RawMetrics{Language: "Brainfuck", Stars: 10}, false)
	if !strings.Contains(out2, "Brainfuck") {
		t.Errorf("unmapped language should keep its name: %q", out2)
	}
}

func TestLanguageBadge(t *testing.T) {
	// Mapped language: glyph only, no name; case-insensitive lookup.
	if got := languageBadge("TypeScript", false); !strings.Contains(got, languageIcons["typescript"].glyph) {
		t.Errorf("mapped language should render its glyph: %q", got)
	}
	if strings.Contains(languageBadge("Go", false), "Go") {
		t.Errorf("mapped language should drop the name in favor of the glyph")
	}
	// ASCII mode: mapped language renders its short tag, not the Nerd Font glyph.
	asciiTS := languageBadge("TypeScript", true)
	if !strings.Contains(asciiTS, languageIcons["typescript"].tag) {
		t.Errorf("ascii mode should render the tag %q: %q", languageIcons["typescript"].tag, asciiTS)
	}
	if strings.Contains(asciiTS, languageIcons["typescript"].glyph) {
		t.Errorf("ascii mode must not render the Nerd Font glyph: %q", asciiTS)
	}
	// Unmapped language with no brand color entry keeps the name (muted dot).
	if got := languageBadge("COBOL", false); !strings.Contains(got, "COBOL") {
		t.Errorf("unmapped language should keep its name: %q", got)
	}
	// Language in languageColors but not languageIcons gets a brand-colored dot
	// plus the name. "Dart" has a brand color (#00b4ab) but no devicon glyph.
	if got := languageBadge("Dart", false); !strings.Contains(got, "Dart") {
		t.Errorf("languageColors language should keep its name: %q", got)
	}
	// The name must not be a languageIcons glyph (i.e. the dot-fallback path was
	// taken, not the glyph path).
	if _, inIcons := languageIcons["dart"]; inIcons {
		t.Fatal("test assumption broken: dart must NOT be in languageIcons")
	}
	if _, inColors := languageColors["dart"]; !inColors {
		t.Fatal("test assumption broken: dart must be in languageColors")
	}
}

func TestJoinEndsOverflow(t *testing.T) {
	// When left+right exceed width, they fall back to single-space separation.
	out := joinEnds("aaaaa", "bbbbb", 6)
	if out != "aaaaa bbbbb" {
		t.Errorf("overflow joinEnds = %q, want single space", out)
	}
	// Normal case pads to width.
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
	// Every verdict carries a "(grade X)" clause; assert on that stable fragment
	// rather than a grade-specific opener phrase.
	if r.Verdict == "" || !strings.Contains(out, "grade") {
		t.Errorf("scorecard hero should contain the verdict:\n%s", out)
	}
}
