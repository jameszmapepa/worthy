package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/worthy/internal/score"
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

	if len(groups[0].items) != 3 || len(groups[1].items) != 2 {
		t.Errorf("group sizes = %d,%d; want 3,2", len(groups[0].items), len(groups[1].items))
	}
}

func TestQuestionsIntegritySectionPresent(t *testing.T) {
	out := stripANSI(renderQuestions(fixedReport(), 100, -1, false))
	if !strings.Contains(out, "Supply-chain integrity") {
		t.Errorf("integrity section header missing from questions view:\n%s", out)
	}
}

func TestQuestionsIntegritySectionContainsSecuritySubs(t *testing.T) {
	out := stripANSI(renderQuestions(fixedReport(), 100, -1, false))
	for _, want := range []string{"CI present", "Workflow safety"} {
		if !strings.Contains(out, want) {
			t.Errorf("integrity section missing %q:\n%s", want, out)
		}
	}
}

func TestQuestionsIntegritySectionIsSelectable(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")

	for range 10 {
		m = press(m, "j")
	}
	if m.selected != 6 {
		t.Errorf("clamp with integrity items: selected=%d, want 6", m.selected)
	}
}

func TestQuestionsIntegritySectionSelectionMarker(t *testing.T) {
	r := fixedReport()

	out := renderQuestions(r, 100, 5, false)
	if !strings.Contains(out, "▸") {
		t.Error("integrity section item at index 5 should show selection marker")
	}
}

func TestQuestionsViewStillSaysTwoQuestions(t *testing.T) {
	out := stripANSI(renderQuestions(fixedReport(), 80, -1, false))
	if !strings.Contains(out, "Two questions") {
		t.Errorf("title must be 'Two questions', got:\n%s", out)
	}
	if strings.Contains(out, "Three questions") {
		t.Errorf("title must NOT be 'Three questions', got:\n%s", out)
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

	_ = renderQuestions(score.Report{}, 80, 0, true)
}

func TestQuestionsDrillJKMovesSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")
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
	m = press(m, "2")
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
	m = press(m, "esc")
	if m.expanded {
		t.Error("esc should collapse")
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
	m = press(m, "3")
	m = press(m, "2")
	if m.selected != 0 || m.expanded {
		t.Errorf("re-entering questions: selected=%d expanded=%v, want 0/false", m.selected, m.expanded)
	}
}

func TestAllSixteenIndicatorsSelectableWithRealScorer(t *testing.T) {
	r := realReport()

	groups := buildQuestionGroups(r)
	integrity := integrityItems(r)
	total := 0
	for _, g := range groups {
		total += len(g.items)
	}
	total += len(integrity)

	if total != 16 {
		t.Fatalf("expected 16 selectable indicators, got %d (activity=%d community=%d security=%d)",
			total,
			func() int {
				for _, g := range groups {
					if g.verdict.Key == "maintained" {
						return len(g.items)
					}
				}
				return 0
			}(),
			func() int {
				for _, g := range groups {
					if g.verdict.Key == "newcomer" {
						return len(g.items)
					}
				}
				return 0
			}(),
			len(integrity),
		)
	}

	m := Model{}
	updated, _ := m.Update(resultMsg{report: r, raw: fixedRaw()})
	m = updated.(Model)
	m = press(m, "2")
	for range 25 {
		m = press(m, "j")
	}
	if m.selected != 15 {
		t.Errorf("all-16 clamp: selected=%d, want 15 (max indicator index)", m.selected)
	}

	for i := range 16 {
		out := renderQuestions(r, 100, i, false)
		if !strings.Contains(out, "▸") {
			t.Errorf("index %d: selection marker missing from rendered output", i)
		}
	}
}

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

func TestLetterGradeThresholds(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{"A on boundary", 85, "A"},
		{"A above", 92.5, "A"},
		{"B on boundary", 70, "B"},
		{"B just below A", 84.9, "B"},
		{"C on boundary", 55, "C"},
		{"C just below B", 69.9, "C"},
		{"D on boundary", 40, "D"},
		{"D just below C", 54.9, "D"},
		{"F just below D", 39.9, "F"},
		{"F at zero", 0, "F"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := score.LetterGrade(tc.value)

			if got != tc.want {
				t.Errorf("score.LetterGrade(%.1f) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
