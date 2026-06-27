package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/worthy/internal/github"
)

func newTestModel() Model {
	c := github.NewClient(github.WithToken(""))
	return New(context.Background(), c, "charmbracelet", "bubbletea")
}

func TestInitialStateIsLoading(t *testing.T) {
	m := newTestModel()
	if m.state != stateLoading {
		t.Errorf("initial state = %v, want loading", m.state)
	}
	if m.Init() == nil {
		t.Error("Init must return a non-nil command (fetch + spinner tick)")
	}
}

func TestResultMsgMovesToLoaded(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(resultMsg{report: fixedReport(), raw: fixedRaw()})
	got := updated.(Model)
	if got.state != stateLoaded {
		t.Errorf("state after success = %v, want loaded", got.state)
	}
	if got.report.Grade != "C" {
		t.Errorf("report not stored: grade = %q", got.report.Grade)
	}
}

func TestResultMsgErrorMovesToErrored(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(resultMsg{err: errors.New("boom")})
	got := updated.(Model)
	if got.state != stateErrored {
		t.Errorf("state after error = %v, want errored", got.state)
	}
	if got.err == nil {
		t.Error("error not stored on model")
	}
}

func TestTabCyclesViews(t *testing.T) {
	m := newTestModel()
	m.state = stateLoaded
	m.report = fixedReport()
	m.raw = fixedRaw()

	for want := 1; want <= 4; want++ {
		next, _ := m.Update(keyPress("tab"))
		m = next.(Model)
		expected := want % 4
		if m.view != expected {
			t.Errorf("after %d tabs view = %d, want %d", want, m.view, expected)
		}
	}
}

func TestNumberKeysSelectViews(t *testing.T) {
	m := newTestModel()
	m.state = stateLoaded
	for _, tc := range []struct {
		key  string
		view int
	}{{"1", 0}, {"2", 1}, {"3", 2}, {"4", 3}} {
		next, _ := m.Update(keyPress(tc.key))
		m = next.(Model)
		if m.view != tc.view {
			t.Errorf("key %q -> view %d, want %d", tc.key, m.view, tc.view)
		}
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c", "esc"} {
		m := newTestModel()
		m.state = stateLoaded
		_, cmd := m.Update(keyPress(key))
		if cmd == nil {
			t.Fatalf("key %q produced no command, want tea.Quit", key)
		}
		if msg := cmd(); !isQuit(msg) {
			t.Errorf("key %q did not produce tea.Quit (got %T)", key, msg)
		}
	}
}

func TestReKeyReturnsToLoading(t *testing.T) {
	m := newTestModel()
	m.state = stateLoaded
	m.report = fixedReport()
	next, cmd := m.Update(keyPress("r"))
	got := next.(Model)
	if got.state != stateLoading {
		t.Errorf("after 'r' state = %v, want loading", got.state)
	}
	if cmd == nil {
		t.Error("'r' must kick off a re-fetch command")
	}
}

func TestViewLoadingShowsSpinnerContext(t *testing.T) {
	m := newTestModel()
	out := m.View().Content
	if !strings.Contains(out, "charmbracelet/bubbletea") {
		t.Errorf("loading view missing repo identity:\n%s", out)
	}
}

func TestViewErroredShowsError(t *testing.T) {
	m := newTestModel()
	m.state = stateErrored
	m.err = errors.New("kaboom")
	out := m.View().Content
	if !strings.Contains(out, "kaboom") {
		t.Errorf("errored view missing message:\n%s", out)
	}
}

func TestRateLimitErrorShowsTokenHint(t *testing.T) {
	m := newTestModel()
	m.state = stateErrored
	m.err = &github.RateLimitError{Endpoint: "/repos/x/y", Limit: 60}
	out := m.View().Content
	if !strings.Contains(out, "GITHUB_TOKEN") {
		t.Errorf("rate-limit view should hint at GITHUB_TOKEN:\n%s", out)
	}
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}

func windowSize(w int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: 24}
}
