package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func sized(t *testing.T, w, h int) Model {
	t.Helper()
	m := loadedModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func TestViewportKeepsSelectionVisible(t *testing.T) {
	m := sized(t, 100, 20)
	for range m.currentSelectableCount() - 1 {
		m = press(m, "j")
	}
	m = press(m, "enter")
	visible := m.viewport.View()
	if !strings.Contains(visible, selectionMarker) {
		t.Errorf("last selected row should be scrolled into view:\n%s", visible)
	}
	if !strings.Contains(visible, "Formula") {
		t.Errorf("drill-down should be scrolled into view:\n%s", visible)
	}
	m = press(m, "esc")
	for range m.currentSelectableCount() - 1 {
		m = press(m, "k")
	}
	if m.viewport.YOffset() != 0 {
		t.Errorf("moving back to the first row should scroll to top, offset = %d", m.viewport.YOffset())
	}
}

func TestViewportMouseWheelScrolls(t *testing.T) {
	m := sized(t, 100, 20)
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	scrolled := next.(Model)
	if got := scrolled.viewport.YOffset(); got == 0 {
		t.Error("wheel down should scroll the body")
	}
}

func TestViewportPageKeysAndIndicator(t *testing.T) {
	m := sized(t, 100, 20)
	if !strings.Contains(stripANSI(m.renderFooter()), "↕") {
		t.Errorf("footer should show a scroll indicator when the body overflows:\n%s", m.renderFooter())
	}
	m = press(m, "pgdown")
	if m.viewport.YOffset() == 0 {
		t.Error("pgdown should scroll")
	}
	m = press(m, "pgup")
	if m.viewport.YOffset() != 0 {
		t.Errorf("pgup should scroll back, offset = %d", m.viewport.YOffset())
	}
	m.view = 3
	m.syncViewport(true)
	before := m.viewport.YOffset()
	m = press(m, "j")
	if m.viewport.YOffset() <= before && m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
		t.Error("j on a non-selectable view should scroll")
	}
}

func TestViewportResetsOnViewSwitch(t *testing.T) {
	m := sized(t, 100, 20)
	m = press(m, "pgdown")
	m = press(m, "tab")
	if m.viewport.YOffset() != 0 {
		t.Errorf("switching view should scroll to top, offset = %d", m.viewport.YOffset())
	}
}

func TestViewEnablesMouseWheel(t *testing.T) {
	m := loadedModel(t)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("mouse cell-motion mode should be on for wheel scrolling")
	}
}
