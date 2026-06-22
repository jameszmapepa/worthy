package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// loadedModel returns a model in the loaded state on the scorecard view, with
// the deterministic fixedReport (7 sub-scores, indices 0..6).
func loadedModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel()
	updated, _ := m.Update(resultMsg{report: fixedReport(), raw: fixedRaw()})
	return updated.(Model)
}

func press(m Model, key string) Model {
	next, _ := m.Update(keyPress(key))
	return next.(Model)
}

func TestDrillJKMovesSelection(t *testing.T) {
	m := loadedModel(t)
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

func TestDrillArrowsMoveSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "down")
	if m.selected != 1 {
		t.Errorf("after down, selected = %d, want 1", m.selected)
	}
	m = press(m, "up")
	if m.selected != 0 {
		t.Errorf("after up, selected = %d, want 0", m.selected)
	}
}

func TestDrillSelectionClampsAtBounds(t *testing.T) {
	m := loadedModel(t)
	// Down past the bottom clamps at count-1 (7 subs -> 6).
	for range 12 {
		m = press(m, "j")
	}
	if m.selected != 6 {
		t.Errorf("selected after over-scroll = %d, want 6", m.selected)
	}
	// Up past the top clamps at 0.
	for range 12 {
		m = press(m, "k")
	}
	if m.selected != 0 {
		t.Errorf("selected after under-scroll = %d, want 0", m.selected)
	}
}

func TestDrillInertWhileLoading(t *testing.T) {
	m := newTestModel() // still loading
	m = press(m, "j")
	if m.selected != 0 {
		t.Errorf("selection must be inert while loading, got %d", m.selected)
	}
	m = press(m, "enter")
	if m.expanded {
		t.Error("expand must be inert while loading")
	}
}

// TestDrillInertOnExplainView confirms selection keys are inert on the Explain
// view (view 4), the one loaded view without drill-down. The scorecard, radar,
// and gauge views are all selectable; explain is not.
func TestDrillInertOnExplainView(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "4") // explain view
	m = press(m, "j")
	if m.selected != 0 {
		t.Errorf("selection must be inert on the explain view, got %d", m.selected)
	}
	m = press(m, "enter")
	if m.expanded {
		t.Error("expand must be inert on the explain view")
	}
}

func TestDrillExpandCollapse(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand")
	}
	m = press(m, "left")
	if m.expanded {
		t.Error("left should collapse")
	}
	m = press(m, "right")
	if !m.expanded {
		t.Error("right should expand")
	}
}

func TestDrillEscCollapsesWhenExpanded(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "enter")
	next, cmd := m.Update(keyPress("esc"))
	m = next.(Model)
	if m.expanded {
		t.Error("esc should collapse an expanded detail")
	}
	if cmd != nil {
		t.Error("esc must not quit while a detail is expanded")
	}
}

func TestDrillEscQuitsWhenNotExpanded(t *testing.T) {
	m := loadedModel(t) // not expanded
	_, cmd := m.Update(keyPress("esc"))
	if cmd == nil {
		t.Fatal("esc should quit when nothing is expanded")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("esc cmd should be tea.Quit when not expanded")
	}
}

func TestEnteringScorecardResetsSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "j")
	m = press(m, "j")
	m = press(m, "enter") // selected=2, expanded
	if m.selected != 2 || !m.expanded {
		t.Fatalf("setup: selected=%d expanded=%v, want 2/true", m.selected, m.expanded)
	}
	m = press(m, "2") // leave scorecard
	m = press(m, "1") // re-enter scorecard
	if m.selected != 0 || m.expanded {
		t.Errorf("re-entering scorecard: selected=%d expanded=%v, want 0/false", m.selected, m.expanded)
	}
}

func TestTabBackToScorecardResetsSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "j")   // selected=1
	m = press(m, "tab") // -> radar
	m = press(m, "tab") // -> gauges
	m = press(m, "tab") // -> explain
	m = press(m, "tab") // -> scorecard
	if m.selected != 0 {
		t.Errorf("tab back to scorecard: selected=%d, want 0", m.selected)
	}
}

func TestDrillRenderShowsDetailWhenExpanded(t *testing.T) {
	r := fixedReport()
	// sub index 1 is commit_recency, with a formula and a linked gate.
	out := renderScorecard(r, 100, 1, true)
	if !strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded detail should show the selected sub-score's Formula")
	}
	if !strings.Contains(out, "stale_or_archived") {
		t.Error("expanded detail should show the linked gate")
	}

	// Collapsed / no-selection render must not show the detail.
	plain := renderScorecard(r, 100, -1, false)
	if strings.Contains(plain, "max(0, 100 − days/365 × 100)") {
		t.Error("collapsed scorecard must not render the formula detail")
	}
}

// TestDrillInertWhileErrored confirms that selection keys are inert when the
// model is in the stateErrored phase (fetch failed). canSelect() gates on
// stateLoaded; this test verifies the errored branch specifically.
func TestDrillInertWhileErrored(t *testing.T) {
	m := newTestModel()
	// Transition to the errored state by delivering a resultMsg with an error.
	next, _ := m.Update(resultMsg{err: fmt.Errorf("network failure")})
	m = next.(Model)

	m = press(m, "j")
	if m.selected != 0 {
		t.Errorf("selection must be inert while errored, got %d", m.selected)
	}
	m = press(m, "enter")
	if m.expanded {
		t.Error("expand must be inert while errored")
	}
}

// TestDrillRenderSelectedButNotExpanded verifies that selecting an indicator
// without expanding it does not leak the detail panel into the output. This
// tests the sel=true, expanded=false combination which is distinct from both
// the highlighted-only path and the expanded-detail path.
func TestDrillRenderSelectedButNotExpanded(t *testing.T) {
	r := fixedReport()
	// Select sub index 1 (commit_recency) but do not expand.
	out := renderScorecard(r, 100, 1, false)
	if strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("selected-but-not-expanded row must not show the formula detail")
	}
	// The selection marker should still appear so the user sees what is focused.
	if !strings.Contains(out, "▸") {
		t.Error("selected-but-not-expanded row should still show the selection marker")
	}
}
