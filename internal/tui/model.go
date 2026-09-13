// Package tui is the Bubble Tea v2 terminal UI for worthy.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
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
	gen    int
	report score.Report
	raw    score.RawMetrics
	err    error
}

type progressMsg struct {
	gen int
	p   metrics.Progress
}

const progressBuffer = 64

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

	viewport viewport.Model
	keys     keyMap
	opener   func(url string) error

	status    string
	statusGen int

	fetchGen    int
	fetchCancel context.CancelFunc
	progress    chan tea.Msg
	stages      []stageStatus
	hasRepo     bool
	revalidate  bool

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
		viewport:  viewport.New(),
		keys:      defaultKeyMap(),
		opener:    openInBrowser,
		width:     80,
		height:    0,
		loadStart: time.Now(),
	}
	for _, o := range opts {
		o(&m)
	}
	m.prepareFetch()
	return m
}

// Init starts the spinner, the background-colour request and the first fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.RequestBackgroundColor, m.fetchCmd())
}

func (m *Model) prepareFetch() {
	if m.fetchCancel != nil {
		m.fetchCancel()
	}
	m.fetchGen++
	m.state = stateLoading
	m.err = nil
	m.hasRepo = false
	m.loadStart = time.Now()
	m.stages = newStages()
	m.progress = make(chan tea.Msg, progressBuffer)
	_, m.fetchCancel = context.WithCancel(m.ctx)
	m.revalidate = m.fetchGen > 1
}

func (m Model) fetchCmd() tea.Cmd {
	return tea.Batch(m.collectCmd(), waitProgress(m.progress))
}

func (m Model) collectCmd() tea.Cmd {
	ctx, cancel := context.WithTimeout(m.ctx, fetchTimeout)
	if m.revalidate {
		ctx = github.ForceRevalidate(ctx)
	}
	client := m.client
	owner, repo, now := m.owner, m.repo, m.now
	gen, ch := m.fetchGen, m.progress
	parentCancel := m.fetchCancel
	return func() tea.Msg {
		defer cancel()
		go func() {
			<-ctx.Done()
			parentCancel()
		}()
		emit := func(p metrics.Progress) {
			select {
			case ch <- progressMsg{gen: gen, p: p}:
			case <-ctx.Done():
			}
		}
		raw, err := metrics.Collect(ctx, client, owner, repo, now, metrics.WithProgress(emit))
		close(ch)
		if err != nil {
			return resultMsg{gen: gen, err: err}
		}
		return resultMsg{gen: gen, report: score.Evaluate(raw), raw: raw}
	}
}

func waitProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// Update handles messages and returns the next model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncViewport(true)
		return m, nil

	case tea.MouseWheelMsg:
		m.syncViewport(false)
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
		return m, nil

	case progressMsg:
		if msg.gen != m.fetchGen {
			return m, nil
		}
		m.applyProgress(msg.p)
		return m, waitProgress(m.progress)

	case resultMsg:
		if msg.gen != m.fetchGen {
			return m, nil
		}
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

	case statusMsg:
		return m, m.setStatus(msg.text)

	case statusExpiredMsg:
		if msg.gen == m.statusGen {
			m.status = ""
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keymap()
	switch {
	case key.Matches(msg, k.Quit):
		m.cancel()
		return m, tea.Quit
	case key.Matches(msg, k.Back):
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
	case key.Matches(msg, k.Help):
		m.helpVisible = !m.helpVisible
		return m, nil
	case key.Matches(msg, k.Next):
		m.view = (m.view + 1) % viewCount
		m.resetSelection()
		return m, nil
	case key.Matches(msg, k.Prev):
		m.view = (m.view - 1 + viewCount) % viewCount
		m.resetSelection()
		return m, nil
	case key.Matches(msg, k.Jump):
		m.view = int(msg.String()[0] - '1')
		m.resetSelection()
		return m, nil
	case key.Matches(msg, k.Refresh):
		m.prepareFetch()
		return m, tea.Batch(m.spinner.Tick, m.fetchCmd())
	case key.Matches(msg, k.Down):
		if m.canSelect() {
			m.moveSelection(1)
		} else {
			m.scroll(func() { m.viewport.ScrollDown(1) })
		}
		return m, nil
	case key.Matches(msg, k.Up):
		if m.canSelect() {
			m.moveSelection(-1)
		} else {
			m.scroll(func() { m.viewport.ScrollUp(1) })
		}
		return m, nil
	case key.Matches(msg, k.Top):
		if m.canSelect() {
			m.moveSelection(-m.selected)
		} else {
			m.scroll(func() { m.viewport.GotoTop() })
		}
		return m, nil
	case key.Matches(msg, k.Bottom):
		if m.canSelect() {
			m.moveSelection(m.currentSelectableCount())
		} else {
			m.scroll(func() { m.viewport.GotoBottom() })
		}
		return m, nil
	case key.Matches(msg, k.PageDown):
		m.scroll(m.viewport.HalfPageDown)
		return m, nil
	case key.Matches(msg, k.PageUp):
		m.scroll(m.viewport.HalfPageUp)
		return m, nil
	case key.Matches(msg, k.Enter):
		if m.canSelect() {
			m.expanded = true
			m.syncViewport(true)
		}
		return m, nil
	case key.Matches(msg, k.Open):
		return m, m.openCmd()
	case key.Matches(msg, k.Copy):
		return m, tea.Batch(m.copyCmd(), m.setStatus("Copied summary to clipboard"))
	}
	return m, nil
}

func (m Model) keymap() keyMap {
	if len(m.keys.Quit.Keys()) == 0 {
		return defaultKeyMap()
	}
	return m.keys
}

func (m *Model) scroll(move func()) {
	m.syncViewport(false)
	move()
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
	m.syncViewport(true)
}

func (m *Model) resetSelection() {
	m.selected = 0
	m.expanded = false
	m.viewport.GotoTop()
	m.syncViewport(true)
}

// View renders the current state.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = m.windowTitle()
	v.ProgressBar = m.progressBar()
	return v
}

func (m Model) windowTitle() string {
	title := "worthy · " + m.owner + "/" + m.repo
	if m.state == stateLoaded && m.report.Grade != "" {
		title += " · Grade " + m.report.Grade
	}
	return title
}

func (m Model) progressBar() *tea.ProgressBar {
	switch m.state {
	case stateLoading:
		if len(m.stages) == 0 {
			return tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
		}
		return tea.NewProgressBar(tea.ProgressBarDefault, m.stagesFinished()*100/len(m.stages))
	case stateErrored:
		return tea.NewProgressBar(tea.ProgressBarError, 100)
	default:
		return nil
	}
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
