// Package tui is the Bubble Tea (v2) terminal UI for repo-health. It runs the
// metrics collection + scoring as an async command, then renders the resulting
// score.Report across three switchable views: a scorecard, a radar, and gauges
// with a commit-trend sparkline.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/metrics"
	"github.com/jameszmapepa/repo-health/internal/score"
)

// state is the model's lifecycle phase.
type state int

const (
	stateLoading state = iota // fetching + scoring
	stateLoaded               // report ready
	stateErrored              // fetch failed
)

// viewCount is the number of switchable views (scorecard, radar, gauges).
const viewCount = 3

// resultMsg carries the outcome of the async collect+evaluate command.
type resultMsg struct {
	report score.Report
	raw    score.RawMetrics
	err    error
}

// Model is the Bubble Tea model for the repo-health TUI.
type Model struct {
	ctx    context.Context
	client *github.Client
	owner  string
	repo   string
	now    time.Time

	state    state
	view     int  // active view index 0..viewCount-1
	selected int  // selected indicator index on the scorecard (flattened)
	expanded bool // whether the selected indicator's detail is expanded
	width    int
	spinner  spinner.Model

	report score.Report
	raw    score.RawMetrics
	err    error
}

// Option configures a Model.
type Option func(*Model)

// WithNow injects the reference time used for time-relative metrics, making the
// fetch deterministic in tests. Defaults to time.Now() at New.
func WithNow(now time.Time) Option {
	return func(m *Model) { m.now = now }
}

// New constructs a Model in the loading state for owner/repo.
func New(ctx context.Context, client *github.Client, owner, repo string, opts ...Option) Model {
	m := Model{
		ctx:     ctx,
		client:  client,
		owner:   owner,
		repo:    repo,
		now:     time.Now(),
		state:   stateLoading,
		spinner: spinner.New(),
		width:   80,
	}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// Init starts the spinner and kicks off the first fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchCmd())
}

// fetchCmd runs metrics.Collect + score.Evaluate off the UI goroutine and
// reports the result as a resultMsg.
func (m Model) fetchCmd() tea.Cmd {
	ctx, client := m.ctx, m.client
	owner, repo, now := m.owner, m.repo, m.now
	return func() tea.Msg {
		raw, err := metrics.Collect(ctx, client, owner, repo, now)
		if err != nil {
			return resultMsg{err: err}
		}
		return resultMsg{report: score.Evaluate(raw), raw: raw}
	}
}

// Update handles messages and returns the next model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case resultMsg:
		if msg.err != nil {
			m.state = stateErrored
			m.err = msg.err
			return m, nil
		}
		m.state = stateLoaded
		m.report = msg.report
		m.raw = msg.raw
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleKey processes key presses: quit, view switching, re-fetch, and (on the
// scorecard) indicator selection + drill-down.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Collapse an open drill-down; otherwise quit.
		if m.canSelect() && m.expanded {
			m.expanded = false
			return m, nil
		}
		return m, tea.Quit
	case "tab":
		m.view = (m.view + 1) % viewCount
		m.syncScorecardSelection()
		return m, nil
	case "1":
		m.view = 0
		m.syncScorecardSelection()
		return m, nil
	case "2":
		m.view = 1
		return m, nil
	case "3":
		m.view = 2
		return m, nil
	case "r":
		m.state = stateLoading
		m.err = nil
		return m, tea.Batch(m.spinner.Tick, m.fetchCmd())
	case "j", "down":
		if m.canSelect() {
			m.moveSelection(1)
		}
		return m, nil
	case "k", "up":
		if m.canSelect() {
			m.moveSelection(-1)
		}
		return m, nil
	case "enter", "right":
		if m.canSelect() {
			m.expanded = true
		}
		return m, nil
	case "left":
		if m.canSelect() {
			m.expanded = false
		}
		return m, nil
	}
	return m, nil
}

// canSelect reports whether indicator selection keys are active: only on the
// loaded scorecard view with at least one indicator. They are inert while
// loading, on error, and on the radar/gauge views.
func (m Model) canSelect() bool {
	return m.state == stateLoaded && m.view == 0 && m.indicatorCount() > 0
}

// indicatorCount is the total number of sub-scores across all categories.
func (m Model) indicatorCount() int {
	n := 0
	for _, c := range m.report.Categories {
		n += len(c.Subs)
	}
	return n
}

// moveSelection shifts the selected indicator by delta, clamped to range.
func (m *Model) moveSelection(delta int) {
	n := m.indicatorCount()
	if n == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= n {
		m.selected = n - 1
	}
}

// syncScorecardSelection resets selection state whenever the scorecard becomes
// the active view, so re-entering it always starts at the top, collapsed.
func (m *Model) syncScorecardSelection() {
	if m.view == 0 {
		m.selected = 0
		m.expanded = false
	}
}

// View renders the current state.
func (m Model) View() tea.View {
	return tea.NewView(m.render())
}

// Run constructs and runs the program to completion (blocking).
func Run(ctx context.Context, client *github.Client, owner, repo string, opts ...Option) error {
	m := New(ctx, client, owner, repo, opts...)
	_, err := tea.NewProgram(m).Run()
	return err
}
