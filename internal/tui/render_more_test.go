package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/score"
)

func TestWithNowOption(t *testing.T) {
	when := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	m := New(context.Background(), github.NewClient(), "o", "r", WithNow(when))
	if !m.now.Equal(when) {
		t.Errorf("WithNow not applied: got %v, want %v", m.now, when)
	}
}

func TestRenderDispatchesAllViews(t *testing.T) {
	m := New(context.Background(), github.NewClient(github.WithToken("")), "o", "r")
	m.state = stateLoaded
	m.report = fixedReport()
	m.raw = fixedRaw()

	for view, marker := range map[int]string{
		0: "Grade",         // scorecard headline
		1: "Two questions", // questions view title
		2: "commit trend",  // gauges sparkline label
	} {
		m.view = view
		out := m.View().Content
		if !strings.Contains(out, marker) {
			t.Errorf("view %d render missing %q:\n%s", view, marker, out)
		}
	}
}

func TestFetchCmdErrorPath(t *testing.T) {
	// metrics.Collect currently returns a nil error for a skeleton; exercise the
	// error branch directly through Update with a synthesized resultMsg.
	m := New(context.Background(), github.NewClient(), "o", "r")
	updated, _ := m.Update(resultMsg{err: errors.New("collect failed")})
	if updated.(Model).state != stateErrored {
		t.Error("error resultMsg should move to errored state")
	}
}

func TestFetchCmdReturnsResultMsgFromCancelledContext(t *testing.T) {
	// A cancelled context makes Collect fail fast without a live network call,
	// so the test stays deterministic and offline while still exercising
	// fetchCmd end-to-end (it must always yield a resultMsg).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := New(ctx, github.NewClient(), "torvalds", "linux")
	msg := m.fetchCmd()()
	if _, ok := msg.(resultMsg); !ok {
		t.Errorf("fetchCmd produced %T, want resultMsg", msg)
	}
}

func TestSeverityGlyphCritical(t *testing.T) {
	g, _ := severityGlyph(score.SeverityCritical)
	if g != glyphCritical {
		t.Errorf("critical glyph = %q, want %q", g, glyphCritical)
	}
}

func TestRenderGatesEmpty(t *testing.T) {
	out := renderGates(nil)
	if !strings.Contains(out, "No gates") {
		t.Errorf("empty gates render = %q", out)
	}
}

func TestRenderGatesCriticalCap(t *testing.T) {
	cap40 := 40.0
	out := renderGates([]score.Gate{
		{Key: "stale_or_archived", Severity: score.SeverityCritical, Title: "Archived", Detail: "dead", CapTo: &cap40},
	})
	if !strings.Contains(out, glyphCritical) || !strings.Contains(out, "caps 40") {
		t.Errorf("critical gate render missing glyph/cap:\n%s", out)
	}
}

func TestRenderSparklineEmpty(t *testing.T) {
	out := renderSparkline(nil, 40)
	if !strings.Contains(out, "no commit") {
		t.Errorf("empty sparkline = %q", out)
	}
}

func TestTruncateShortString(t *testing.T) {
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate short = %q, want unchanged", got)
	}
	if got := truncate("hello world", 5); len([]rune(got)) != 5 {
		t.Errorf("truncate long = %q (len %d), want 5 runes", got, len([]rune(got)))
	}
}

func TestWindowSizeUpdatesWidth(t *testing.T) {
	m := New(context.Background(), github.NewClient(), "o", "r")
	updated, _ := m.Update(windowSize(120))
	if updated.(Model).width != 120 {
		t.Errorf("width after resize = %d, want 120", updated.(Model).width)
	}
}
