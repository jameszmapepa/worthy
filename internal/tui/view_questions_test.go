package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/repo-health/internal/score"
)

func TestQuestionsViewContent(t *testing.T) {
	out := stripANSI(renderQuestions(fixedReport(), 80, -1, false))
	for _, w := range []string{
		"Two questions",
		"Will it last?",
		"Will my PR land?",
		"Commit frequency",
		"License",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("questions view missing %q in:\n%s", w, out)
		}
	}
}

func TestQuestionsGroupsSortedDescending(t *testing.T) {
	groups := buildQuestionGroups(fixedReport())
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].verdict.Key != "maintained" || groups[1].verdict.Key != "newcomer" {
		t.Fatalf("group order = %q,%q; want maintained,newcomer", groups[0].verdict.Key, groups[1].verdict.Key)
	}
	for _, g := range groups {
		for i := 1; i < len(g.items); i++ {
			if g.items[i-1].sub.Value < g.items[i].sub.Value {
				t.Errorf("%s not sorted desc: %.0f before %.0f",
					g.verdict.Key, g.items[i-1].sub.Value, g.items[i].sub.Value)
			}
		}
	}
	// maintained = activity(3) + security(2) = 5 subs; newcomer = community(2).
	if len(groups[0].items) != 5 || len(groups[1].items) != 2 {
		t.Errorf("group sizes = %d,%d; want 5,2", len(groups[0].items), len(groups[1].items))
	}
}

func TestQuestionsSelectionMarker(t *testing.T) {
	out := renderQuestions(fixedReport(), 100, 0, false)
	if !strings.Contains(out, "▸") {
		t.Error("selected indicator should show the selection marker")
	}
}

func TestQuestionsDrillShowsDetailWhenExpanded(t *testing.T) {
	r := fixedReport()
	idx := flatIndexOfSub(t, r, "commit_recency")
	out := renderQuestions(r, 100, idx, true)
	if !strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded detail should show the selected indicator's Formula")
	}
	if !strings.Contains(out, "stale_or_archived") {
		t.Error("expanded detail should show the linked gate")
	}
	plain := renderQuestions(r, 100, -1, false)
	if strings.Contains(plain, "max(0, 100 − days/365 × 100)") {
		t.Error("collapsed view must not render the formula detail")
	}
}

func TestQuestionsExpandedWithNoSelectionIsNoOp(t *testing.T) {
	out := renderQuestions(fixedReport(), 100, -1, true)
	if strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded with no selection (-1) must not render a detail panel")
	}
}

func TestQuestionsGateMappedToReferencingQuestion(t *testing.T) {
	// stale_or_archived is referenced by commit_recency/release_cadence, which
	// live in the maintained group; when triggered its badge must render.
	r := fixedReport()
	capVal := 60.0
	r.Gates = append(r.Gates, score.Gate{
		Key: "stale_or_archived", Severity: score.SeverityWarn,
		Title: "Stale repo", Detail: "no recent commits", CapTo: &capVal,
	})
	out := renderQuestions(r, 100, -1, false)
	if !strings.Contains(out, "Stale repo") {
		t.Errorf("a triggered gate referenced by a displayed indicator should render:\n%s", out)
	}
}

func TestQuestionsLeftoverGateRendered(t *testing.T) {
	// vanity_stars is referenced by no sub-score; it must still render so
	// repo-level signals are not silently dropped.
	out := renderQuestions(fixedReport(), 100, -1, false)
	if !strings.Contains(out, "Stars outpace engagement") {
		t.Errorf("an unreferenced (leftover) gate should still render:\n%s", out)
	}
}

func TestQuestionsEmptyReportNoPanic(t *testing.T) {
	out := renderQuestions(score.Report{}, 80, -1, false)
	if !strings.Contains(out, "no indicators") {
		t.Errorf("empty report should render the empty state, got:\n%s", out)
	}
	// Selection args on an empty report must not panic.
	_ = renderQuestions(score.Report{}, 80, 0, true)
}

// --- model-level drill-down on the questions view (view 1) --------------------

func TestQuestionsDrillJKMovesSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // questions
	if m.selected != 0 {
		t.Fatalf("initial selected = %d, want 0", m.selected)
	}
	m = press(m, "j")
	if m.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", m.selected)
	}
	m = press(m, "k")
	if m.selected != 0 {
		t.Errorf("after k, selected = %d, want 0", m.selected)
	}
}

func TestQuestionsDrillClampsToIndicatorCount(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // 7 indicators -> max index 6
	for range 12 {
		m = press(m, "j")
	}
	if m.selected != 6 {
		t.Errorf("over-scroll selected = %d, want 6", m.selected)
	}
}

func TestQuestionsDrillExpandCollapse(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand")
	}
	m = press(m, "left")
	if m.expanded {
		t.Error("left should collapse")
	}
}

func TestQuestionsSelectionResetsOnViewSwitch(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")
	m = press(m, "j")
	m = press(m, "enter")
	if m.selected != 1 || !m.expanded {
		t.Fatalf("setup: selected=%d expanded=%v, want 1/true", m.selected, m.expanded)
	}
	m = press(m, "3") // gauges
	m = press(m, "2") // back to questions
	if m.selected != 0 || m.expanded {
		t.Errorf("re-entering questions: selected=%d expanded=%v, want 0/false", m.selected, m.expanded)
	}
}

// flatIndexOfSub locates a sub-score's flattened index in the questions view's
// display order (question-grouped, value-sorted) via the production grouping,
// so detail-drill tests do not hardcode a sort-dependent position.
func flatIndexOfSub(t *testing.T, r score.Report, key string) int {
	t.Helper()
	idx := 0
	for _, g := range buildQuestionGroups(r) {
		for _, it := range g.items {
			if it.sub.Key == key {
				return idx
			}
			idx++
		}
	}
	t.Fatalf("sub %q not found in questions layout", key)
	return -1
}
