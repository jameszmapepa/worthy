// Package tui is the Bubble Tea v2 terminal UI for worthy.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/metrics"
	"github.com/jameszmapepa/worthy/internal/score"
)

type state int

const (
	stateLoading state = iota
	stateLoaded
	stateErrored
)

const viewCount = 4

const fetchTimeout = 60 * time.Second

type resultMsg struct {
	report score.Report
	raw    score.RawMetrics
	err    error
}

// Model is the Bubble Tea model for the worthy TUI.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *github.Client
	owner  string
	repo   string
	now    time.Time

	state       state
	view        int
	selected    int
	expanded    bool
	helpVisible bool
	asciiIcons  bool
	width       int
	height      int
	loadStart   time.Time
	spinner     spinner.Model

	report score.Report
	raw    score.RawMetrics
	err    error
}

// Option configures a Model.
type Option func(*Model)

// WithNow injects the reference time for time-relative metrics; defaults to time.Now() at construction.
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
		cancel:    func() {},
		client:    client,
		owner:     owner,
		repo:      repo,
		now:       time.Now(),
		state:     stateLoading,
		spinner:   spinner.New(),
		width:     80,
		height:    0,
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

func (m Model) fetchCmd() tea.Cmd {
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

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":

		m.cancel()
		return m, tea.Quit
	case "esc":

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

		m.helpVisible = !m.helpVisible
		return m, nil
	case "tab", "right", "l":

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

func (m Model) canSelect() bool {
	return m.state == stateLoaded && m.currentSelectableCount() > 0
}

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

func (m Model) indicatorCount() int {
	n := 0
	for _, c := range m.report.Categories {
		n += len(c.Subs)
	}
	return n
}

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

func (m *Model) resetSelection() {
	m.selected = 0
	m.expanded = false
}

// View renders the current state.
func (m Model) View() tea.View {
	return tea.NewView(m.render())
}

// Run constructs and runs the TUI program to completion, blocking until quit.
func Run(ctx context.Context, client *github.Client, owner, repo string, opts ...Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := New(ctx, client, owner, repo, opts...)
	m.cancel = cancel
	_, err := tea.NewProgram(m).Run()
	return err
}
