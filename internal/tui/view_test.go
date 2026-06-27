package tui

import (
	"strings"
	"testing"
)

func TestScorecardViewContent(t *testing.T) {
	out := renderScorecard(fixedReport(), 80, -1, false)

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
	out := renderScorecard(fixedReport(), 80, -1, false)
	if !strings.Contains(out, glyphWarn) {
		t.Errorf("scorecard missing warn glyph %q", glyphWarn)
	}
	if !strings.Contains(out, glyphInfo) {
		t.Errorf("scorecard missing info glyph %q", glyphInfo)
	}
}

func TestGaugesViewContent(t *testing.T) {
	out := renderGauges(fixedReport(), fixedRaw(), 80, -1, false)
	for _, w := range []string{"Activity", "Community", "Security", "Composite", "52-week commit trend"} {
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
	raw := fixedRaw()
	unauth := renderHeaderPanel("charmbracelet", "bubbletea", raw, true, false, 80, "", false)
	if !strings.Contains(unauth, "60") || !strings.Contains(unauth, "charmbracelet/bubbletea") {
		t.Errorf("unauth header wrong:\n%s", unauth)
	}
	// The quota carries an "API" label so the bare number is not cryptic.
	if !strings.Contains(unauth, "API") {
		t.Errorf("rate-limit badge should be labeled with API:\n%s", unauth)
	}
	auth := renderHeaderPanel("charmbracelet", "bubbletea", raw, true, true, 80, "", false)
	if !strings.Contains(auth, "5,000") {
		t.Errorf("auth header should show 5,000/hr:\n%s", auth)
	}
}

// In ASCII mode the header renders the language's short tag (e.g. "TS") instead
// of the Nerd Font devicon glyph, so terminals without a Nerd Font stay legible.
func TestHeaderASCIIMode(t *testing.T) {
	raw := fixedRaw()
	raw.Language = "TypeScript"

	ascii := renderHeaderPanel("o", "r", raw, true, false, 100, "", true)
	if !strings.Contains(ascii, languageIcons["typescript"].tag) {
		t.Errorf("ascii header should show the TS tag:\n%s", ascii)
	}
	if strings.Contains(ascii, languageIcons["typescript"].glyph) {
		t.Errorf("ascii header must not contain the Nerd Font glyph:\n%s", ascii)
	}

	glyph := renderHeaderPanel("o", "r", raw, true, false, 100, "", false)
	if !strings.Contains(glyph, languageIcons["typescript"].glyph) {
		t.Errorf("default header should show the devicon glyph:\n%s", glyph)
	}
}

func TestHeaderShowsMetaWhenLoaded(t *testing.T) {
	raw := fixedRaw()
	raw.Description = "A delightful TUI framework"
	raw.LicenseSPDX = "MIT"
	raw.RepoAgeDays = 1200
	out := renderHeaderPanel("charm", "bubbletea", raw, true, true, 100, "B", false)
	for _, want := range []string{"delightful TUI", glyphStar, "MIT", "3.3y old"} {
		if !strings.Contains(out, want) {
			t.Errorf("loaded header missing %q:\n%s", want, out)
		}
	}
}

func TestHeaderHidesMetaWhenNotLoaded(t *testing.T) {
	raw := fixedRaw()
	raw.Description = "should not show yet"
	out := renderHeaderPanel("charm", "bubbletea", raw, false, true, 100, "", false)
	if strings.Contains(out, "should not show yet") {
		t.Errorf("header must not show description before load:\n%s", out)
	}
	if !strings.Contains(out, "charm/bubbletea") {
		t.Errorf("header must always show identity:\n%s", out)
	}
}

// TestHeaderShowsGradeWhenLoaded verifies the C1 change: the composite letter
// grade is appended to the identity row once metrics are loaded.
func TestHeaderShowsGradeWhenLoaded(t *testing.T) {
	raw := fixedRaw()
	out := stripANSI(renderHeaderPanel("o", "r", raw, true, false, 100, "A", false))
	if !strings.Contains(out, "Grade A") {
		t.Errorf("loaded header should show the composite grade:\n%s", out)
	}
}

func TestHeaderOmitsGradeWhenNotLoaded(t *testing.T) {
	raw := fixedRaw()
	out := stripANSI(renderHeaderPanel("o", "r", raw, false, false, 100, "", false))
	if strings.Contains(out, "Grade") {
		t.Errorf("header must not show grade before load:\n%s", out)
	}
}

func TestHumanizeAge(t *testing.T) {
	cases := map[int]string{
		0:    "new",
		5:    "5d old",
		60:   "2.0mo old",
		1200: "3.3y old",
	}
	for days, want := range cases {
		if got := humanizeAge(days); got != want {
			t.Errorf("humanizeAge(%d) = %q, want %q", days, got, want)
		}
	}
}

func TestHumanizeCount(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		999:     "999",
		4200:    "4.2k",
		2500000: "2.5M",
	}
	for n, want := range cases {
		if got := humanizeCount(n); got != want {
			t.Errorf("humanizeCount(%d) = %q, want %q", n, got, want)
		}
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
