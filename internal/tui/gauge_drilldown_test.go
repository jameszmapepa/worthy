package tui

import (
	"strings"
	"testing"

	"github.com/jameszmapepa/worthy/internal/score"
)

func TestGaugeDrillJKClampsToCategories(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "3")
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
	m = press(m, "3")
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand on gauges")
	}
	m = press(m, "esc")
	if m.expanded {
		t.Error("esc should collapse on gauges")
	}
}

func TestCanSelectOnQuestionsAndGauges(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "2")
	if !m.canSelect() {
		t.Error("questions view should be selectable when loaded")
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
	m.view = 3
	if strings.Contains(m.renderFooter(), "drill") {
		t.Error("footer on the explain view should not show the drill hint")
	}
}

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

	_ = renderQuestions(score.Report{}, 80, 0, true)
	_ = renderGauges(score.Report{}, score.RawMetrics{}, 80, 0, true)
}
