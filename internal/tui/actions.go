package tui

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
)

// statusTTL is how long a transient footer message stays visible.
const statusTTL = 2 * time.Second

type statusMsg struct {
	text string
}

type statusExpiredMsg struct {
	gen int
}

func (m Model) repoURL() string {
	return "https://github.com/" + url.PathEscape(m.owner) + "/" + url.PathEscape(m.repo)
}

// summary is the one-line result copied by y.
func (m Model) summary() string {
	if m.state != stateLoaded {
		return m.owner + "/" + m.repo + " · " + m.repoURL()
	}
	r := m.report
	return fmt.Sprintf("%s/%s · Grade %s (%.1f/100) · Will it last? %s · Will my PR land? %s · %s",
		m.owner, m.repo, r.Grade, r.AdjustedComposite, r.Maintained.Grade, r.Contributable.Grade, m.repoURL())
}

func (m *Model) setStatus(text string) tea.Cmd {
	m.statusGen++
	m.status = text
	gen := m.statusGen
	return tea.Tick(statusTTL, func(time.Time) tea.Msg { return statusExpiredMsg{gen: gen} })
}

func (m Model) copyCmd() tea.Cmd {
	return tea.SetClipboard(m.summary())
}

func (m Model) openCmd() tea.Cmd {
	open, u := m.opener, m.repoURL()
	return func() tea.Msg {
		if err := open(u); err != nil {
			return statusMsg{text: "Could not open browser: " + err.Error()}
		}
		return statusMsg{text: "Opened " + u}
	}
}

// openInBrowser hands the URL to the platform opener without waiting for it.
// The binary is fixed per platform and u is built from the validated
// owner/repo pair, so there is no user-controlled command text.
func openInBrowser(u string) error {
	// The opener is detached, so a background context is the honest choice:
	// there is nothing to cancel once it has been handed the URL.
	ctx := context.Background()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", u) //nolint:gosec // fixed binary, escaped URL
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", u) //nolint:gosec // fixed binary, escaped URL
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", u) //nolint:gosec // fixed binary, escaped URL
	}
	return cmd.Start()
}
