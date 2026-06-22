package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// render produces the full screen string for the current model state.
func (m Model) render() string {
	header := renderHeaderPanel(
		m.owner, m.repo, m.raw,
		m.state == stateLoaded, m.client.Authenticated(), m.width,
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

	return header + "\n\n" + body + "\n\n" + m.renderFooter()
}

// renderLoading shows the spinner alongside the repo being fetched.
func (m Model) renderLoading() string {
	return fmt.Sprintf("%s Fetching %s/%s …", m.spinner.View(), m.owner, m.repo)
}

// renderError shows the failure, with a token hint on rate-limit errors.
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
	return b.String()
}

// renderActiveView dispatches to the selected view renderer.
func (m Model) renderActiveView() string {
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

// renderFooter shows the view tabs and key hints.
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
	hint := "tab/1-4 switch · r refresh · q quit"
	if m.canSelect() {
		hint = "↑↓ select · enter drill · " + hint
	}
	keys := mutedStyle.Render(hint)
	return tabs + "    " + keys
}

// isRateLimit reports whether err is (or wraps) a github rate-limit error.
func isRateLimit(err error) bool {
	var rl *github.RateLimitError
	return errors.As(err, &rl)
}
