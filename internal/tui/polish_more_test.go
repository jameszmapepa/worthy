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
	out := metaRow(raw)
	if !strings.Contains(out, "no license") {
		t.Errorf("meta row should show 'no license' when none: %s", out)
	}
	if !strings.Contains(out, "1.1y old") {
		t.Errorf("meta row should show age: %s", out)
	}
}

func TestMetaRowWithLanguage(t *testing.T) {
	raw := score.RawMetrics{Language: "Go", LicenseSPDX: "MIT", Stars: 5000}
	out := metaRow(raw)
	if !strings.Contains(out, "Go") || !strings.Contains(out, "5.0k") {
		t.Errorf("meta row missing language/stars: %s", out)
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
	// The verdict's grade phrase should be embedded in the hero block.
	if r.Verdict == "" || !strings.Contains(out, "health") {
		t.Errorf("scorecard hero should contain the verdict:\n%s", out)
	}
}
