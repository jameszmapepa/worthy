package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// realReport builds a full 14-sub-score Report through the score engine so the
// radar is exercised against the real indicator set, not a trimmed fixture.
func realReport() score.Report {
	return score.Evaluate(score.RawMetrics{
		CommitsLast52Weeks:            []int{5, 6, 7, 8, 6, 5, 9, 7, 6, 8, 5, 7},
		DaysSinceLastPush:             10,
		RepoAgeDays:                   800,
		OpenIssues:                    20,
		ClosedIssues:                  180,
		OpenPRs:                       5,
		MergedPRs:                     60,
		ClosedUnmergedPRs:             8,
		MedianIssueFirstResponseHours: 30,
		NewcomerPRsMerged:             5,
		NewcomerPRsClosedUnmerged:     3,
		TopContributorRecentShare:     0.5,
		ContributorCount:              8,
		ReleaseCount:                  6,
		DaysSinceLastRelease:          120,
		HasReadme:                     true,
		HasContributing:               true,
		HasLicense:                    true,
		LicenseSPDX:                   "Apache-2.0",
		HasCI:                         true,
		HasSecurityPolicy:             false,
		WorkflowsFetched:              true,
		Stars:                         3000,
		Watchers:                      400,
	})
}

func TestRadarLabelsEverySubScore(t *testing.T) {
	r := realReport()
	out := renderRadar(r, 80, -1, false)

	for _, c := range r.Categories {
		for _, s := range c.Subs {
			// The indicator list abbreviates labels to 20 runes; assert on that.
			want := truncate(s.Label, 20)
			if !strings.Contains(out, want) {
				t.Errorf("radar missing indicator label %q\n---\n%s", want, out)
			}
		}
	}
}

func TestRadarShowsIndicatorCountAndLegend(t *testing.T) {
	out := renderRadar(realReport(), 80, -1, false)
	if !strings.Contains(out, "14 indicators") {
		t.Errorf("radar should announce the indicator count:\n%s", out)
	}
	for _, cat := range []string{"Activity", "Community", "Security"} {
		if !strings.Contains(out, cat) {
			t.Errorf("radar legend missing %q", cat)
		}
	}
}

func TestRadarContainsBrailleGlyphs(t *testing.T) {
	out := renderRadar(realReport(), 80, -1, false)
	hasBraille := false
	for _, r := range out {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("radar plot should contain braille glyphs:\n%s", out)
	}
}

func TestRadarEmptyReportDoesNotPanic(t *testing.T) {
	out := renderRadar(score.Report{}, 80, -1, false)
	if !strings.Contains(out, "no indicators") {
		t.Errorf("empty radar should note no indicators, got:\n%s", out)
	}
}

func TestBrailleCanvasSetAndRender(t *testing.T) {
	cv := newBrailleCanvas(2, 2)
	cv.set(0, 0, nil)                // top-left dot of cell (0,0)
	cv.line(0, 0, 3, 7, colorAccent) // diagonal across the surface
	out := cv.render()
	if out == "" {
		t.Fatal("canvas render is empty")
	}
	// Out-of-bounds set must be ignored, not panic.
	cv.set(-1, -1, nil)
	cv.set(999, 999, nil)
}
