package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// fixedReport has 7 sub-scores across 3 categories:
//   Activity:  commit_frequency(0), commit_recency(1), release_cadence(2)
//   Community: issue_responsiveness(3), license(4)
//   Security:  ci_present(5), workflow_safety(6)
// Index 1 (commit_recency) carries a Formula and a linked gate, matching the
// scorecard drill-down fixtures.

// --- Radar (view 1) drill-down ------------------------------------------------

func TestRadarDrillJKMovesSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // radar
	if m.selected != 0 {
		t.Fatalf("radar initial selected = %d, want 0", m.selected)
	}
	m = press(m, "j")
	if m.selected != 1 {
		t.Errorf("radar after j, selected = %d, want 1", m.selected)
	}
	m = press(m, "k")
	if m.selected != 0 {
		t.Errorf("radar after k, selected = %d, want 0", m.selected)
	}
}

func TestRadarDrillClampsToIndicatorCount(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // radar; 7 indicators -> max index 6
	for range 12 {
		m = press(m, "j")
	}
	if m.selected != 6 {
		t.Errorf("radar over-scroll selected = %d, want 6", m.selected)
	}
}

func TestRadarDrillExpandCollapse(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // radar
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand on radar")
	}
	m = press(m, "left")
	if m.expanded {
		t.Error("left should collapse on radar")
	}
}

func TestRadarRenderShowsDetailWhenExpanded(t *testing.T) {
	r := fixedReport()
	// Index 1 is commit_recency, with a formula and a linked gate.
	out := renderRadar(r, 100, 1, true)
	if !strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded radar should show the selected indicator's Formula")
	}
	if !strings.Contains(out, "stale_or_archived") {
		t.Error("expanded radar should show the linked gate")
	}

	plain := renderRadar(r, 100, -1, false)
	if strings.Contains(plain, "max(0, 100 − days/365 × 100)") {
		t.Error("collapsed radar must not render the formula detail")
	}
}

func TestRadarRenderShowsSelectionMarker(t *testing.T) {
	out := renderRadar(fixedReport(), 100, 1, false)
	if !strings.Contains(out, "▸") {
		t.Error("selected radar indicator should show the selection marker")
	}
}

func TestRadarSelectionResetsOnViewSwitch(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2") // radar
	m = press(m, "j")
	m = press(m, "enter") // selected=1, expanded
	if m.selected != 1 || !m.expanded {
		t.Fatalf("setup: selected=%d expanded=%v, want 1/true", m.selected, m.expanded)
	}
	m = press(m, "3") // gauges
	m = press(m, "2") // back to radar
	if m.selected != 0 || m.expanded {
		t.Errorf("re-entering radar: selected=%d expanded=%v, want 0/false", m.selected, m.expanded)
	}
}

// --- Gauges (view 2) drill-down -----------------------------------------------

func TestGaugeDrillJKClampsToCategories(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "3") // gauges; 3 categories -> max index 2 (composite not selectable)
	if m.selected != 0 {
		t.Fatalf("gauges initial selected = %d, want 0", m.selected)
	}
	for range 6 {
		m = press(m, "j")
	}
	if m.selected != 2 {
		t.Errorf("gauges over-scroll selected = %d, want 2 (3 categories)", m.selected)
	}
	m = press(m, "k")
	if m.selected != 1 {
		t.Errorf("gauges after k, selected = %d, want 1", m.selected)
	}
}

func TestGaugeDrillExpandShowsCategoryDetail(t *testing.T) {
	r := fixedReport()
	raw := fixedRaw()
	// Category 0 is Activity; its sub labels include "Commit frequency",
	// which the static gauge view never shows.
	out := renderGauges(r, raw, 100, 0, true)
	if !strings.Contains(out, "Commit frequency") {
		t.Error("expanded Activity gauge should list its sub-score labels")
	}

	plain := renderGauges(r, raw, 100, -1, false)
	if strings.Contains(plain, "Commit frequency") {
		t.Error("collapsed gauges must not list category sub-scores")
	}
}

func TestGaugeDrillShowsSelectionMarker(t *testing.T) {
	out := renderGauges(fixedReport(), fixedRaw(), 100, 0, false)
	if !strings.Contains(out, "▸") {
		t.Error("selected gauge category should show the selection marker")
	}
}

func TestGaugeDrillExpandCollapseViaKeys(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "3") // gauges
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand on gauges")
	}
	m = press(m, "left")
	if m.expanded {
		t.Error("left should collapse on gauges")
	}
}

// --- Shared selection plumbing ------------------------------------------------

func TestCanSelectOnRadarAndGauges(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")
	if !m.canSelect() {
		t.Error("radar view should be selectable when loaded")
	}
	m = press(m, "3")
	if !m.canSelect() {
		t.Error("gauges view should be selectable when loaded")
	}
	m = press(m, "4")
	if m.canSelect() {
		t.Error("explain view should not be selectable")
	}
}

func TestFooterHintShowsDrillOnSelectableViews(t *testing.T) {
	m := loadedModel(t)
	for _, v := range []int{0, 1, 2} {
		m.view = v
		if !strings.Contains(m.renderFooter(), "drill") {
			t.Errorf("footer on view %d should show the drill hint", v)
		}
	}
	m.view = 3 // explain: no selection
	if strings.Contains(m.renderFooter(), "drill") {
		t.Error("footer on the explain view should not show the drill hint")
	}
}

// --- Empty-report and no-selection safety -------------------------------------

func TestEmptyReportSelectionInert(t *testing.T) {
	m := newTestModel()
	upd, _ := m.Update(resultMsg{report: score.Report{}, raw: score.RawMetrics{}})
	m = upd.(Model)
	for _, v := range []int{0, 1, 2} {
		m.view = v
		if m.canSelect() {
			t.Errorf("empty report: view %d must not be selectable", v)
		}
	}
	// Render paths must not panic when handed an empty report with selection args.
	_ = renderRadar(score.Report{}, 80, 0, true)
	_ = renderGauges(score.Report{}, score.RawMetrics{}, 80, 0, true)
}

func TestRadarExpandedWithNoSelectionIsNoOp(t *testing.T) {
	out := renderRadar(fixedReport(), 100, -1, true)
	if strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded radar with no selection (-1) must not render a detail panel")
	}
}
