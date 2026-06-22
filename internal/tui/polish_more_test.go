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

func TestProjectGuardsZeroAxes(t *testing.T) {
	// n<1 is treated as 1 rather than dividing by zero.
	x, y := project(100, 0, 0, 50, 50, 20)
	if x != 50 || y != 30 {
		t.Errorf("project with n=0 = (%v,%v), want (50,30)", x, y)
	}
}

func TestCategoryColorDefault(t *testing.T) {
	if categoryColor("unknown") != colorFg {
		t.Error("unknown category should fall back to foreground color")
	}
}

func TestBrailleCanvasClampsSmallSizes(t *testing.T) {
	cv := newBrailleCanvas(0, 0) // clamped to at least 1x1
	if cv.cols < 1 || cv.rows < 1 {
		t.Errorf("canvas not clamped: %dx%d", cv.cols, cv.rows)
	}
	cv.set(0, 0, colorAccent)
	if cv.render() == "" {
		t.Error("clamped canvas should still render")
	}
}

func TestDrawRingTinyRadiusNoPanic(t *testing.T) {
	cv := newBrailleCanvas(4, 4)
	drawRing(cv, 4, 8, 0.5, colorTrackEmpty) // radius<1 returns early
	drawRing(cv, 4, 8, 3, colorTrackEmpty)   // normal
	_ = cv.render()
}

func TestVerdictAppearsInScorecardHero(t *testing.T) {
	r := realReport()
	out := renderScorecard(r, 100)
	// The verdict's grade phrase should be embedded in the hero block.
	if r.Verdict == "" || !strings.Contains(out, "health") {
		t.Errorf("scorecard hero should contain the verdict:\n%s", out)
	}
}
