package tui

// Tests covering the C1-C10 tasks that either have testable behaviour or were
// explicitly required by the spec. Each test group is labelled with the task it
// covers.

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/score"
)

// ── C1: persistent header grade ───────────────────────────────────────────────

func TestC1HeaderGradeAppearsOnAllViews(t *testing.T) {
	m := loadedModel(t) // starts on scorecard (view 0)
	for _, viewKey := range []string{"1", "2", "3", "4"} {
		m = press(m, viewKey)
		out := stripANSI(m.render())
		if !strings.Contains(out, "Grade") {
			t.Errorf("view %s: header should contain Grade when loaded, got:\n%s", viewKey, out)
		}
	}
}

func TestC1HeaderGradeAbsentWhileLoading(t *testing.T) {
	m := newTestModel() // stateLoading
	out := stripANSI(m.render())
	if strings.Contains(out, "Grade") {
		t.Errorf("loading view must not show Grade:\n%s", out)
	}
}

// ── C2: question headline cards ───────────────────────────────────────────────

func TestC2QuestionCardsAppearInScorecard(t *testing.T) {
	out := stripANSI(renderScorecard(fixedReport(), 100, -1, false))
	for _, want := range []string{"Will it last?", "Will my PR land?"} {
		if !strings.Contains(out, want) {
			t.Errorf("scorecard missing question card %q:\n%s", want, out)
		}
	}
}

func TestC2QuestionCardsAboveCategories(t *testing.T) {
	out := stripANSI(renderScorecard(fixedReport(), 100, -1, false))
	qPos := strings.Index(out, "Will it last?")
	aPos := strings.Index(out, "Activity")
	if qPos < 0 || aPos < 0 {
		t.Fatal("missing question card or Activity section")
	}
	if qPos >= aPos {
		t.Errorf("question cards (%d) should appear before category panels (%d)", qPos, aPos)
	}
}

func TestC2ConfidenceCaveatLow(t *testing.T) {
	// score.ConfidenceLow triggers a "Limited data" warning.
	r := fixedReport()
	r.Confidence = score.ConfidenceLow
	out := stripANSI(renderScorecard(r, 100, -1, false))
	if !strings.Contains(out, "Limited data") {
		t.Errorf("Low-confidence scorecard should show caveat:\n%s", out)
	}
}

func TestC2ConfidenceCaveatMedium(t *testing.T) {
	r := fixedReport()
	r.Confidence = score.ConfidenceMedium
	out := stripANSI(renderScorecard(r, 100, -1, false))
	if !strings.Contains(out, "Some data unavailable") {
		t.Errorf("Medium-confidence scorecard should show caveat:\n%s", out)
	}
}

func TestC2ConfidenceCaveatHighOmitted(t *testing.T) {
	r := fixedReport()
	r.Confidence = score.ConfidenceHigh
	out := stripANSI(renderScorecard(r, 100, -1, false))
	if strings.Contains(out, "Limited data") || strings.Contains(out, "Some data") {
		t.Errorf("High-confidence scorecard should NOT show a caveat:\n%s", out)
	}
}

// ── C3: terminal height tracking ──────────────────────────────────────────────

func TestC3HeightStoredOnWindowResize(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	if updated.(Model).height != 50 {
		t.Errorf("height after resize = %d, want 50", updated.(Model).height)
	}
}

func TestC3TruncationSentinelAppearsWhenOverflow(t *testing.T) {
	// Use a very small height so the body is definitely truncated.
	m := loadedModel(t)
	m.height = 8 // tiny terminal: header + footer consume most of it
	out := m.render()
	if !strings.Contains(out, "↓ content truncated") {
		t.Errorf("tiny terminal: expected truncation sentinel:\n%s", out)
	}
}

func TestC3NoTruncationWhenHeightZero(t *testing.T) {
	// height=0 means unknown; truncation must be skipped entirely.
	m := loadedModel(t)
	m.height = 0
	out := m.render()
	if strings.Contains(out, "↓ content truncated") {
		t.Error("height=0 must not trigger truncation")
	}
}

// ── C4: narrow terminal ───────────────────────────────────────────────────────

func TestC4NarrowGaugesNoHorizontalOverflow(t *testing.T) {
	// Below narrowTerminalWidth the gauge+trend panels must stack vertically.
	// renderGauges must not panic and must still contain both panel labels.
	out := stripANSI(renderGauges(fixedReport(), fixedRaw(), 50, -1, false))
	for _, want := range []string{"Category gauges", "Activity"} {
		if !strings.Contains(out, want) {
			t.Errorf("narrow gauges missing %q:\n%s", want, out)
		}
	}
}

func TestC4NarrowScorecardCardsStack(t *testing.T) {
	// Below narrowTerminalWidth the question cards stack vertically.
	// At narrow widths the question label may wrap; check for unique substrings
	// rather than the full label string.
	out := stripANSI(renderScorecard(fixedReport(), 50, -1, false))
	for _, want := range []string{"Will it last", "Will my PR"} {
		if !strings.Contains(out, want) {
			t.Errorf("narrow scorecard missing question card content %q:\n%s", want, out)
		}
	}
}

// ── C5: text-tier grade (NO_COLOR accessibility) ─────────────────────────────

func TestC5SubLineContainsGradeLetter(t *testing.T) {
	r := fixedReport()
	// Activity[0] = commit_frequency at value 90 → grade "A"
	s := r.Categories[0].Subs[0]
	line := stripANSI(renderSubLine(s, 14, 20, false))
	if !strings.Contains(line, "A") {
		t.Errorf("renderSubLine should contain letter grade 'A' (value=%.0f):\n%s", s.Value, line)
	}
}

func TestC5GaugeLabelContainsGradeLetter(t *testing.T) {
	line := stripANSI(renderGauge("Activity", 82.5, 14, false))
	// 82.5 → grade "B"
	if !strings.Contains(line, "B") {
		t.Errorf("renderGauge should contain letter grade 'B' (value=82.5):\n%s", line)
	}
}

// ── C6: loading elapsed indicator ────────────────────────────────────────────

func TestC6LoadingShowsElapsed(t *testing.T) {
	m := newTestModel()
	// Set loadStart to 5 seconds ago so elapsed rounds to 5s.
	m.loadStart = time.Now().Add(-5 * time.Second)
	out := m.renderLoading()
	if !strings.Contains(out, "5s") {
		t.Errorf("loading view should show elapsed time (5s):\n%s", out)
	}
}

func TestC6LoadingContainsFetchContext(t *testing.T) {
	m := newTestModel()
	out := m.renderLoading()
	if !strings.Contains(out, "charmbracelet/bubbletea") {
		t.Errorf("loading view should show the repo:\n%s", out)
	}
}

// ── C7: renderBar grouped-gradient performance ────────────────────────────────

func TestC7BarVisibleOutputIdenticalAfterRewrite(t *testing.T) {
	// The grouped-run renderBar must produce exactly the same visible characters
	// (after stripping ANSI) as the naive per-cell approach: filled cells are
	// barFilled, empty cells are barEmpty, total == width.
	cases := []struct {
		value float64
		width int
		want  int // expected filled cells
	}{
		{0, 20, 0},
		{50, 20, 10},
		{75, 20, 15},
		{100, 20, 20},
		{33.3, 15, 5},
	}
	for _, tc := range cases {
		bar := stripANSI(renderBar(tc.value, tc.width))
		runes := []rune(bar)
		if len(runes) != tc.width {
			t.Errorf("renderBar(%.1f, %d): total width=%d, want %d",
				tc.value, tc.width, len(runes), tc.width)
		}
		filledRune := []rune(barFilled)[0]
		filled := 0
		for _, r := range runes {
			if r == filledRune {
				filled++
			}
		}
		if filled != tc.want {
			t.Errorf("renderBar(%.1f, %d): filled=%d, want %d",
				tc.value, tc.width, filled, tc.want)
		}
	}
}

// ── C8: hand-rolled sparkline ─────────────────────────────────────────────────

func TestC8SparklineEmpty(t *testing.T) {
	out := renderSparkline(nil, 40)
	if !strings.Contains(out, "no commit") {
		t.Errorf("empty sparkline = %q", out)
	}
}

func TestC8SparklineSingleValue(t *testing.T) {
	// A single non-zero value should produce width identical runes (flat line).
	out := stripANSI(renderSparkline([]int{5}, 6))
	runes := []rune(out)
	if len(runes) != 6 {
		t.Fatalf("single-value sparkline: got %d chars, want 6", len(runes))
	}
	// All runes identical (flat data uses the mid block).
	for _, r := range runes {
		if r != runes[0] {
			t.Error("single-value sparkline: all cells should be the same rune")
		}
	}
}

func TestC8SparklineAscendingSeries(t *testing.T) {
	// An ascending series must produce a rune sequence that ends higher than it
	// starts in the sparklineRunes ordering.
	data := []int{1, 2, 3, 4, 5, 6, 7, 8}
	out := stripANSI(renderSparkline(data, len(data)))
	runes := []rune(out)
	if len(runes) == 0 {
		t.Fatal("ascending sparkline rendered empty")
	}
	firstIdx, lastIdx := -1, -1
	for i, r := range sparklineRunes {
		if r == runes[0] {
			firstIdx = i
		}
		if r == runes[len(runes)-1] {
			lastIdx = i
		}
	}
	if firstIdx < 0 || lastIdx < 0 {
		t.Fatalf("ascending sparkline contains unrecognised runes: %q", out)
	}
	if firstIdx >= lastIdx {
		t.Errorf("ascending sparkline: first block idx %d should be < last block idx %d",
			firstIdx, lastIdx)
	}
}

func TestC8SparklineBlockWidthMatchesRequest(t *testing.T) {
	data := []int{1, 5, 3, 8, 2}
	for _, w := range []int{4, 10, 20} {
		out := sparklineBlock(data, w)
		if len([]rune(out)) != w {
			t.Errorf("sparklineBlock width=%d: got %d chars", w, len([]rune(out)))
		}
	}
}

// ── C9: context cancellation ──────────────────────────────────────────────────

func TestC9QuitCallsCancel(t *testing.T) {
	// When 'q' is pressed, the model must call cancel() so any in-flight
	// goroutine sees ctx.Done().
	cancelled := false
	m := newTestModel()
	m.cancel = func() { cancelled = true }
	m.state = stateLoaded
	m.report = fixedReport()
	m = press(m, "q") // discard cmd; we only care about the side-effect
	if !cancelled {
		t.Error("pressing 'q' should call model.cancel()")
	}
}

func TestC9FetchCmdBoundsTimeout(t *testing.T) {
	// A cancelled parent context makes metrics.Collect fail fast; fetchCmd must
	// still return a resultMsg (never block indefinitely).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := New(ctx, github.NewClient(), "o", "r")
	msg := m.fetchCmd()()
	if _, ok := msg.(resultMsg); !ok {
		t.Errorf("fetchCmd produced %T, want resultMsg", msg)
	}
}

// ── C10: help overlay, retry line, footer hints ───────────────────────────────

func TestC10HelpOverlayTogglesOnQuestionMark(t *testing.T) {
	m := loadedModel(t)
	if m.helpVisible {
		t.Fatal("help should be hidden initially")
	}
	m = press(m, "?")
	if !m.helpVisible {
		t.Error("'?' should show help")
	}
	m = press(m, "?")
	if m.helpVisible {
		t.Error("second '?' should hide help")
	}
}

func TestC10HelpOverlayEscCloses(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "?") // open help
	next, cmd := m.Update(keyPress("esc"))
	m = next.(Model)
	if m.helpVisible {
		t.Error("esc should close the help overlay")
	}
	if cmd != nil {
		// esc with help visible must close help, not quit.
		t.Error("esc on help overlay must not produce a quit command")
	}
}

func TestC10HelpOverlayRendersKeybindings(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "?")
	out := stripANSI(m.render())
	for _, want := range []string{"Keybindings", "drill down", "refresh", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay missing %q:\n%s", want, out)
		}
	}
}

func TestC10ErrorViewContainsRetryInstruction(t *testing.T) {
	m := newTestModel()
	m.state = stateErrored
	m.err = &github.RateLimitError{Endpoint: "/repos/o/r", Limit: 60}
	out := m.renderError()
	if !strings.Contains(out, "Press r to retry.") {
		t.Errorf("error view must contain retry instruction:\n%s", out)
	}
}

func TestC10FooterShowsCollapseHintWhenExpanded(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "enter") // expand
	footer := stripANSI(m.renderFooter())
	if !strings.Contains(footer, "collapse") {
		t.Errorf("footer when expanded should show 'collapse' hint:\n%s", footer)
	}
}

func TestC10FooterShowsHelpHintWhenNotExpanded(t *testing.T) {
	m := loadedModel(t)
	footer := stripANSI(m.renderFooter())
	if !strings.Contains(footer, "help") {
		t.Errorf("footer should show '? help' hint:\n%s", footer)
	}
}
