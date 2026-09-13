package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/worthy/internal/metrics"
	"github.com/jameszmapepa/worthy/internal/score"
)

func TestLoadingViewListsStages(t *testing.T) {
	m := newTestModel()
	out := m.render()
	for _, want := range []string{"Scoring charmbracelet/bubbletea", "Repository", "Commit activity", "0/11"} {
		if !strings.Contains(out, want) {
			t.Errorf("loading view missing %q in:\n%s", want, out)
		}
	}
}

func TestProgressUpdatesStageAndHeader(t *testing.T) {
	m := newTestModel()
	gen := m.fetchGen
	hdr := score.RawMetrics{Stars: 4200, Description: "A powerful little TUI framework", LicenseSPDX: "MIT"}

	next, cmd := m.Update(progressMsg{gen: gen, p: metrics.Progress{Stage: metrics.StageRepository, State: metrics.StageDone, Repo: &hdr}})
	got := next.(Model)
	if !got.hasRepo || got.raw.Stars != 4200 {
		t.Fatalf("header metadata not applied: hasRepo=%v stars=%d", got.hasRepo, got.raw.Stars)
	}
	if cmd == nil {
		t.Error("progress must re-arm the wait command")
	}
	out := got.render()
	if !strings.Contains(out, "4.2k") || !strings.Contains(out, "A powerful little TUI framework") {
		t.Errorf("header should show stars and description while loading:\n%s", out)
	}
	if !strings.Contains(out, "1/11") {
		t.Errorf("loading view should count the finished stage:\n%s", out)
	}

	next, _ = got.Update(progressMsg{gen: gen, p: metrics.Progress{Stage: metrics.StageCommits, State: metrics.StageRetrying, Attempt: 2}})
	out = next.(Model).render()
	if !strings.Contains(out, "retry 2") {
		t.Errorf("retrying stage should explain the 202 wait:\n%s", out)
	}

	next, _ = next.(Model).Update(progressMsg{gen: gen, p: metrics.Progress{Stage: metrics.StageCommunity, State: metrics.StageDegraded}})
	out = next.(Model).render()
	if !strings.Contains(out, "unavailable") {
		t.Errorf("degraded stage should be flagged:\n%s", out)
	}
}

func TestStaleProgressAndResultIgnored(t *testing.T) {
	m := newTestModel()
	old := m.fetchGen
	next, _ := m.Update(keyPress("r"))
	got := next.(Model)
	if got.fetchGen == old {
		t.Fatal("r must start a new fetch generation")
	}
	hdr := score.RawMetrics{Stars: 1}
	next, cmd := got.Update(progressMsg{gen: old, p: metrics.Progress{Stage: metrics.StageRepository, State: metrics.StageDone, Repo: &hdr}})
	if next.(Model).hasRepo || cmd != nil {
		t.Error("stale progress must be dropped without re-arming")
	}
	next, _ = got.Update(resultMsg{gen: old, report: fixedReport(), raw: fixedRaw()})
	if next.(Model).state != stateLoading {
		t.Error("stale result must not move the model out of loading")
	}
	next, _ = got.Update(resultMsg{gen: got.fetchGen, report: fixedReport(), raw: fixedRaw()})
	if next.(Model).state != stateLoaded {
		t.Error("current-generation result must load")
	}
}

func TestWaitProgressReturnsNilOnClosedChannel(t *testing.T) {
	ch := make(chan tea.Msg)
	close(ch)
	if got := waitProgress(ch)(); got != nil {
		t.Errorf("closed channel should yield nil, got %v", got)
	}
}

func TestRefreshForcesRevalidation(t *testing.T) {
	m := newTestModel()
	if m.revalidate {
		t.Fatal("first fetch should use the cache")
	}
	next, _ := m.Update(keyPress("r"))
	if !next.(Model).revalidate {
		t.Error("r should force revalidation on the next fetch")
	}
}
