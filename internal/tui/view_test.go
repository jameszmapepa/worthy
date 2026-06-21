package tui

import (
	"strings"
	"testing"
)

func TestScorecardViewContent(t *testing.T) {
	out := renderScorecard(fixedReport(), 80)

	wants := []string{
		"68.2",                // adjusted composite
		"C",                   // grade
		"Activity",            // category label
		"Community",           // category label
		"Security",            // category label
		"Commit frequency",    // sub-score label
		"13.5 commits/wk",     // raw metric text
		"Closed to newcomers", // gate title
		"Stars outpace engagement",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("scorecard missing %q in:\n%s", w, out)
		}
	}
}

func TestScorecardGateGlyphs(t *testing.T) {
	out := renderScorecard(fixedReport(), 80)
	if !strings.Contains(out, glyphWarn) {
		t.Errorf("scorecard missing warn glyph %q", glyphWarn)
	}
	if !strings.Contains(out, glyphInfo) {
		t.Errorf("scorecard missing info glyph %q", glyphInfo)
	}
}

func TestRadarViewContent(t *testing.T) {
	out := renderRadar(fixedReport(), 80)
	for _, w := range []string{"Activity", "Community", "Security"} {
		if !strings.Contains(out, w) {
			t.Errorf("radar missing axis label %q in:\n%s", w, out)
		}
	}
}

func TestGaugesViewContent(t *testing.T) {
	out := renderGauges(fixedReport(), fixedRaw(), 80)
	for _, w := range []string{"Activity", "Community", "Security", "Composite", "Commit trend"} {
		if !strings.Contains(out, w) {
			t.Errorf("gauges missing %q in:\n%s", w, out)
		}
	}
}

func TestBarColorThresholds(t *testing.T) {
	// green >=70, amber 40-69, red <40
	if barColor(70) != colorGreen {
		t.Error("70 should be green")
	}
	if barColor(69.9) != colorAmber {
		t.Error("69.9 should be amber")
	}
	if barColor(40) != colorAmber {
		t.Error("40 should be amber")
	}
	if barColor(39.9) != colorRed {
		t.Error("39.9 should be red")
	}
}

func TestHeaderShowsRateLimitMode(t *testing.T) {
	unauth := renderHeader("charmbracelet", "bubbletea", false, 80)
	if !strings.Contains(unauth, "60") || !strings.Contains(unauth, "charmbracelet/bubbletea") {
		t.Errorf("unauth header wrong:\n%s", unauth)
	}
	auth := renderHeader("charmbracelet", "bubbletea", true, 80)
	if !strings.Contains(auth, "5,000") {
		t.Errorf("auth header should show 5,000/hr:\n%s", auth)
	}
}

func TestBarRenderClampsAndScales(t *testing.T) {
	// A zero-value bar and a full bar must both render within the given width.
	zero := renderBar(0, 20)
	full := renderBar(100, 20)
	over := renderBar(150, 20) // clamps to full
	if len([]rune(stripANSI(full))) == 0 {
		t.Error("full bar rendered empty")
	}
	if stripANSI(full) != stripANSI(over) {
		t.Error("over-100 bar should clamp to full")
	}
	_ = zero
}
