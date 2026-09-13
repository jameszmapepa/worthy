package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/metrics"
)

func TestNoLineOverflows(t *testing.T) {
	widths := []int{50, 60, 70, 80, 100, 120, 160}
	for _, w := range widths {
		for _, r := range []struct {
			name string
			m    Model
		}{
			{"loaded", loadedAt(w)},
			{"loading", loadingAt(w)},
			{"errored", erroredAt(w)},
		} {
			m := r.m
			for view := range viewCount {
				m.view = view
				for _, expanded := range []bool{false, true} {
					m.expanded = expanded
					m.selected = 0
					if expanded && m.canSelect() {
						m.selected = m.currentSelectableCount() - 1
					}
					assertFits(t, r.name+"/view", view, w, m.render())
				}
			}
			m.helpVisible = true
			assertFits(t, r.name+"/help", 0, w, m.render())
		}
	}
}

func assertFits(t *testing.T, name string, view, width int, out string) {
	t.Helper()
	for i, line := range strings.Split(out, "\n") {
		if lw := lipgloss.Width(line); lw > width {
			t.Errorf("%s %d at width %d: line %d is %d wide:\n%s", name, view, width, i, lw, line)
		}
	}
}

func loadedAt(w int) Model {
	m := newTestModel()
	m.state = stateLoaded
	m.report = fixedReport()
	m.raw = fixedRaw()
	m.raw.Description = strings.Repeat("a fairly long description of the project ", 4)
	m.width = w
	return m
}

func loadingAt(w int) Model {
	m := newTestModel()
	m.width = w
	m.applyProgress(metrics.Progress{Stage: metrics.StageCommits, State: metrics.StageRetrying, Attempt: 3})
	m.applyProgress(metrics.Progress{Stage: metrics.StageCommunity, State: metrics.StageDegraded})
	return m
}

func erroredAt(w int) Model {
	m := newTestModel()
	m.state = stateErrored
	m.err = errors.New("github rate limit exhausted (limit 60) on /repos/x/y; resets in 41m (at 3:04PM) with a long tail so the line has to wrap")
	m.width = w
	return m
}

func TestFooterStacksWhenNarrow(t *testing.T) {
	m := loadedAt(80)
	if n := strings.Count(m.renderFooter(), "\n"); n != 1 {
		t.Errorf("footer at 80 cols should stack tabs over hints, got %d newlines", n)
	}
	m = loadedAt(160)
	if n := strings.Count(m.renderFooter(), "\n"); n != 0 {
		t.Errorf("footer at 160 cols should be one line, got %d newlines", n)
	}
}

func TestGateDetailWrapsInsteadOfOverflowing(t *testing.T) {
	r := fixedReport()
	out := renderGates(r.Gates, 60)
	assertFits(t, "gates", 0, 60, out)
	if !strings.Contains(out, "Closed to newcomers") {
		t.Errorf("gate title lost while wrapping:\n%s", out)
	}
}

func TestRawColumnDroppedWhenNarrow(t *testing.T) {
	if got := rawBudgetFor(44, 10); got != 0 {
		t.Errorf("rawBudget at 44 cols = %d, want 0 (dropped)", got)
	}
	if got := rawBudgetFor(96, 28); got != 96-subLineOverhead-28 {
		t.Errorf("rawBudget at 96 cols = %d", got)
	}
}
