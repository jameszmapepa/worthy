package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/worthy/internal/metrics"
)

func TestWindowTitleNamesRepoAndGrade(t *testing.T) {
	m := newTestModel()
	if got := m.View().WindowTitle; got != "worthy · charmbracelet/bubbletea" {
		t.Errorf("loading title = %q", got)
	}
	m = loadedModel(t)
	if got := m.View().WindowTitle; got != "worthy · charmbracelet/bubbletea · Grade C" {
		t.Errorf("loaded title = %q", got)
	}
}

func TestProgressBarTracksStages(t *testing.T) {
	m := newTestModel()
	pb := m.View().ProgressBar
	if pb == nil || pb.State != tea.ProgressBarDefault || pb.Value != 0 {
		t.Fatalf("loading progress bar = %+v", pb)
	}
	for _, st := range metrics.Stages[:5] {
		m.applyProgress(metrics.Progress{Stage: st, State: metrics.StageDone})
	}
	if pb = m.View().ProgressBar; pb.Value != 41 {
		t.Errorf("5/12 stages should be 41%%, got %d", pb.Value)
	}
	if loaded := loadedModel(t); loaded.View().ProgressBar != nil {
		t.Error("loaded state must clear the progress bar")
	}
	m.state = stateErrored
	m.err = errors.New("boom")
	if pb = m.View().ProgressBar; pb == nil || pb.State != tea.ProgressBarError {
		t.Errorf("errored state should show an error bar, got %+v", pb)
	}
}
