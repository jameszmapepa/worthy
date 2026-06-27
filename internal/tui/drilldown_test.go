package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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

	for range 12 {
		m = press(m, "j")
	}
	if m.selected != 6 {
		t.Errorf("selected after over-scroll = %d, want 6", m.selected)
	}

	for range 12 {
		m = press(m, "k")
	}
	if m.selected != 0 {
		t.Errorf("selected after under-scroll = %d, want 0", m.selected)
	}
}

func TestDrillInertWhileLoading(t *testing.T) {
	m := newTestModel()
	m = press(m, "j")
	if m.selected != 0 {
		t.Errorf("selection must be inert while loading, got %d", m.selected)
	}
	m = press(m, "enter")
	if m.expanded {
		t.Error("expand must be inert while loading")
	}
}

func TestDrillInertOnExplainView(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "4")
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
	m = press(m, "esc")
	if m.expanded {
		t.Error("esc should collapse")
	}
	m = press(m, "enter")
	if !m.expanded {
		t.Error("enter should expand again")
	}
}

func TestArrowsCycleViews(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "right")
	if m.view != 1 {
		t.Errorf("right: view = %d; want 1", m.view)
	}
	m = press(m, "left")
	if m.view != 0 {
		t.Errorf("left: view = %d; want 0", m.view)
	}
	m = press(m, "left")
	if m.view != viewCount-1 {
		t.Errorf("left wrap: view = %d; want %d", m.view, viewCount-1)
	}
	m = press(m, "right")
	if m.view != 0 {
		t.Errorf("right wrap: view = %d; want 0", m.view)
	}
}

func TestArrowSwitchResetsSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "j")
	m = press(m, "enter")
	m = press(m, "right")
	if m.selected != 0 || m.expanded {
		t.Errorf("after right: selected=%d expanded=%v; want 0/false", m.selected, m.expanded)
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
	m := loadedModel(t)
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
	m = press(m, "enter")
	if m.selected != 2 || !m.expanded {
		t.Fatalf("setup: selected=%d expanded=%v, want 2/true", m.selected, m.expanded)
	}
	m = press(m, "2")
	m = press(m, "1")
	if m.selected != 0 || m.expanded {
		t.Errorf("re-entering scorecard: selected=%d expanded=%v, want 0/false", m.selected, m.expanded)
	}
}

func TestTabBackToScorecardResetsSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "j")
	m = press(m, "tab")
	m = press(m, "tab")
	m = press(m, "tab")
	m = press(m, "tab")
	if m.selected != 0 {
		t.Errorf("tab back to scorecard: selected=%d, want 0", m.selected)
	}
}

func TestDrillRenderShowsDetailWhenExpanded(t *testing.T) {
	r := fixedReport()

	out := renderScorecard(r, 100, 1, true)
	if !strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("expanded detail should show the selected sub-score's Formula")
	}
	if !strings.Contains(out, "stale_or_archived") {
		t.Error("expanded detail should show the linked gate")
	}

	plain := renderScorecard(r, 100, -1, false)
	if strings.Contains(plain, "max(0, 100 − days/365 × 100)") {
		t.Error("collapsed scorecard must not render the formula detail")
	}
}

func TestDrillInertWhileErrored(t *testing.T) {
	m := newTestModel()

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

func TestDrillRenderSelectedButNotExpanded(t *testing.T) {
	r := fixedReport()

	out := renderScorecard(r, 100, 1, false)
	if strings.Contains(out, "max(0, 100 − days/365 × 100)") {
		t.Error("selected-but-not-expanded row must not show the formula detail")
	}

	if !strings.Contains(out, "▸") {
		t.Error("selected-but-not-expanded row should still show the selection marker")
	}
}
