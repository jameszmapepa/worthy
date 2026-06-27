package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// footerReservedLines is the number of terminal lines consumed by the footer
// block (footer line itself) plus the two "\n\n" separators below the body and
// the two "\n\n" above it. Used by truncateBody to compute available body lines.
const footerReservedLines = 5

// render produces the full screen string for the current model state.
func (m Model) render() string {
	// C1: pass the loaded grade so the header shows it on every view.
	grade := ""
	if m.state == stateLoaded {
		grade = m.report.Grade
	}
	header := renderHeaderPanel(
		m.owner, m.repo, m.raw,
		m.state == stateLoaded, m.client.Authenticated(), m.width, grade, m.asciiIcons,
	)

	var body string
	switch m.state {
	case stateLoading:
		body = m.renderLoading()
	case stateErrored:
		body = m.renderError()
	default:
		body = m.renderActiveView()
	}

	// C3: trim body when it would overflow the terminal so content does not
	// silently spill into scrollback. Only active when height is known (>0).
	if m.height > 0 {
		body = m.truncateBody(header, body)
	}

	return header + "\n\n" + body + "\n\n" + m.renderFooter()
}

// truncateBody trims body to the lines available after reserving space for the
// header, separators, and footer. A muted sentinel is appended when lines are
// cut so the user knows content was hidden (C3).
func (m Model) truncateBody(header, body string) string {
	headerLines := strings.Count(header, "\n") + 1
	available := m.height - headerLines - footerReservedLines
	if available < 1 {
		available = 1
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= available {
		return body
	}
	trimmed := lines[:available-1]
	return strings.Join(trimmed, "\n") + "\n" + mutedStyle.Render("↓ content truncated")
}

// renderLoading shows the spinner alongside the repo being fetched, with an
// elapsed-time indicator so a multi-second fetch does not look frozen (C6).
// The spinner already ticks every ~100ms, driving re-renders that pick up the
// current time.Since(loadStart) without a separate ticker goroutine.
func (m Model) renderLoading() string {
	elapsed := time.Since(m.loadStart).Round(time.Second)
	return fmt.Sprintf("%s Fetching %s/%s … (%s)", m.spinner.View(), m.owner, m.repo, elapsed)
}

// renderError shows the failure with a token hint on rate-limit errors and an
// explicit retry instruction so the action is discoverable (C10).
func (m Model) renderError() string {
	var b strings.Builder
	b.WriteString(errStyle.Render("Could not score " + m.owner + "/" + m.repo))
	b.WriteString("\n\n")
	b.WriteString(m.err.Error())
	if isRateLimit(m.err) {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render(
			"Tip: set a GITHUB_TOKEN to lift the limit to 5,000 requests/hour."))
	}
	// C10: explicit retry hint so the user does not have to guess the key.
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Press r to retry."))
	return b.String()
}

// renderActiveView dispatches to the selected view renderer, checking the help
// overlay first (C10).
func (m Model) renderActiveView() string {
	// C10: help overlay takes priority over the regular view body.
	if m.helpVisible {
		return renderHelp(m.width)
	}
	switch m.view {
	case 1:
		return renderQuestions(m.report, m.width, m.selected, m.expanded)
	case 2:
		return renderGauges(m.report, m.raw, m.width, m.selected, m.expanded)
	case 3:
		return renderExplain(m.report, m.width)
	default:
		return renderScorecard(m.report, m.width, m.selected, m.expanded)
	}
}

// renderFooter shows the view tabs and key hints. When a drill-down is
// expanded the collapse key is surfaced explicitly (C10). The ? key for the
// help overlay is always shown.
func (m Model) renderFooter() string {
	names := []string{"1 Scorecard", "2 Questions", "3 Gauges", "4 Explain"}
	parts := make([]string, len(names))
	for i, n := range names {
		if i == m.view && m.state == stateLoaded {
			parts[i] = titleStyle.Render("[" + n + "]")
		} else {
			parts[i] = mutedStyle.Render(" " + n + " ")
		}
	}
	tabs := strings.Join(parts, " ")

	var hint string
	switch {
	case m.helpVisible:
		hint = "? close help · q quit"
	case m.canSelect() && m.expanded:
		// C10: when a drill-down is open, show the collapse key prominently.
		hint = "esc collapse · ←→ switch view · r refresh · q quit"
	case m.canSelect():
		hint = "↑↓ select · enter drill · ←→ switch view · r refresh · ? help · q quit"
	default:
		hint = "←→ switch view · r refresh · ? help · q quit"
	}
	keys := mutedStyle.Render(hint)
	return tabs + "    " + keys
}

// isRateLimit reports whether err is (or wraps) a github rate-limit error.
func isRateLimit(err error) bool {
	var rl *github.RateLimitError
	return errors.As(err, &rl)
}
