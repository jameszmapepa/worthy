// Package tui is the Bubble Tea (v2) terminal UI for repo-health. It runs the
// metrics collection + scoring as an async command, then renders the resulting
// score.Report across four switchable views: a scorecard, a two-question
// breakdown, gauges with a commit-trend sparkline, and an explain view.
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

// viewCount is the number of switchable views (scorecard, questions, gauges,
// explain).
const viewCount = 4

// fetchTimeout is the maximum time the fetch goroutine is allowed to run
// before it is cancelled and an error is surfaced. A hung GitHub API response
// should never cause an invisible infinite hang (C9).
const fetchTimeout = 60 * time.Second

// resultMsg carries the outcome of the async collect+evaluate command.
type resultMsg struct {
	report score.Report
	raw    score.RawMetrics
	err    error
}

// Model is the Bubble Tea model for the repo-health TUI.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc // cancels the Run-level context on quit (C9)
	client *github.Client
	owner  string
	repo   string
	now    time.Time

	state       state
	view        int  // active view index 0..viewCount-1
	selected    int  // selected item index; meaning is view-dependent
	expanded    bool // whether the selected item's detail panel is expanded
	helpVisible bool // whether the keybinding help overlay is shown (C10)
	asciiIcons  bool // render language as an ASCII tag instead of a Nerd Font glyph
	width       int
	height      int // terminal height from tea.WindowSizeMsg (C3)
	loadStart   time.Time
	spinner     spinner.Model

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

// WithASCIIIcons selects the ASCII-tag language badge (e.g. "TS") instead of the
// Nerd Font devicon glyph, for terminals without a Nerd Font installed.
func WithASCIIIcons(ascii bool) Option {
	return func(m *Model) { m.asciiIcons = ascii }
}

// New constructs a Model in the loading state for owner/repo.
func New(ctx context.Context, client *github.Client, owner, repo string, opts ...Option) Model {
	m := Model{
		ctx:       ctx,
		cancel:    func() {}, // no-op default; Run replaces with a real cancel
		client:    client,
		owner:     owner,
		repo:      repo,
		now:       time.Now(),
		state:     stateLoading,
		spinner:   spinner.New(),
		width:     80,
		height:    0, // 0 means unknown; truncation is skipped until we know
		loadStart: time.Now(),
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
// reports the result as a resultMsg. The fetch is bounded by fetchTimeout so a
// hung GitHub API never causes an invisible hang (C9).
func (m Model) fetchCmd() tea.Cmd {
	// Derive a timeout child of the model's context so the fetch stops when
	// either the user quits (Run-level cancel) or the timeout fires.
	ctx, cancel := context.WithTimeout(m.ctx, fetchTimeout)
	client := m.client
	owner, repo, now := m.owner, m.repo, m.now
	return func() tea.Msg {
		defer cancel()
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
		// C3: track both width and height so render() can truncate long views.
		m.width = msg.Width
		m.height = msg.Height
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

// handleKey processes key presses: quit, view switching, help overlay, re-fetch,
// and (on the selectable views) item selection + drill-down.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// C9: signal the fetch goroutine to stop before quitting.
		m.cancel()
		return m, tea.Quit
	case "esc":
		// Collapse an open drill-down or help overlay; otherwise quit.
		if m.helpVisible {
			m.helpVisible = false
			return m, nil
		}
		if m.canSelect() && m.expanded {
			m.expanded = false
			return m, nil
		}
		m.cancel()
		return m, tea.Quit
	case "?":
		// C10: toggle keybinding help overlay.
		m.helpVisible = !m.helpVisible
		return m, nil
	case "tab", "right", "l":
		// Cycle to the next view. left/right arrows are dedicated view toggles;
		// drill-down expand/collapse now lives on enter/esc so the arrows are
		// free to navigate views from any view, selected or not.
		m.view = (m.view + 1) % viewCount
		m.resetSelection()
		return m, nil
	case "shift+tab", "left", "h":
		m.view = (m.view - 1 + viewCount) % viewCount
		m.resetSelection()
		return m, nil
	case "1":
		m.view = 0
		m.resetSelection()
		return m, nil
	case "2":
		m.view = 1
		m.resetSelection()
		return m, nil
	case "3":
		m.view = 2
		m.resetSelection()
		return m, nil
	case "4":
		m.view = 3
		m.resetSelection()
		return m, nil
	case "r":
		m.state = stateLoading
		m.err = nil
		m.loadStart = time.Now()
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
	case "enter":
		if m.canSelect() {
			m.expanded = true
		}
		return m, nil
	}
	return m, nil
}

// canSelect reports whether selection keys are active: on the loaded scorecard,
// questions, and gauge views, each with at least one selectable item. They are inert
// while loading, on error, and on the explain view.
func (m Model) canSelect() bool {
	return m.state == stateLoaded && m.currentSelectableCount() > 0
}

// currentSelectableCount is the number of selectable items in the active view.
// The scorecard and questions views select individual indicators (the flattened sub-score
// list); the gauge view selects whole categories. Other views select nothing.
func (m Model) currentSelectableCount() int {
	switch m.view {
	case 0, 1:
		return m.indicatorCount()
	case 2:
		return len(m.report.Categories)
	default:
		return 0
	}
}

// indicatorCount is the total number of sub-scores across all categories.
func (m Model) indicatorCount() int {
	n := 0
	for _, c := range m.report.Categories {
		n += len(c.Subs)
	}
	return n
}

// moveSelection shifts the selected item by delta, clamped to the active view's
// selectable range.
func (m *Model) moveSelection(delta int) {
	n := m.currentSelectableCount()
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

// resetSelection clears selection state on every view switch, so entering any
// view always starts at the top, collapsed. selected indexes a different domain
// per view (indicators vs categories), so it must not persist across switches.
func (m *Model) resetSelection() {
	m.selected = 0
	m.expanded = false
}

// View renders the current state.
func (m Model) View() tea.View {
	return tea.NewView(m.render())
}

// Run constructs and runs the program to completion (blocking). A cancellable
// child context is derived from ctx and stored on the model so pressing q or
// ctrl+c cancels any in-flight fetch goroutine before the program exits (C9).
func Run(ctx context.Context, client *github.Client, owner, repo string, opts ...Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // also fires on normal exit / panic

	m := New(ctx, client, owner, repo, opts...)
	m.cancel = cancel
	_, err := tea.NewProgram(m).Run()
	return err
}
