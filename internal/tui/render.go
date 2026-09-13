package tui

import (
	"errors"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jameszmapepa/worthy/internal/github"
)

// footerGap is the blank lines between body and footer.
const footerGap = 2

func (m Model) render() string {
	grade := ""
	if m.state == stateLoaded {
		grade = m.report.Grade
	}
	header := renderHeaderPanel(
		m.owner, m.repo, m.raw,
		m.state == stateLoaded || m.hasRepo, m.client.Authenticated(), m.client.RateInfo(), m.width, grade, m.asciiIcons,
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

	footer := m.renderFooter()
	if m.height > 0 {
		body = m.truncateBody(header, body, footer)
	}

	return header + "\n\n" + body + "\n\n" + footer
}

func (m Model) truncateBody(header, body, footer string) string {
	headerLines := strings.Count(header, "\n") + 1
	footerLines := strings.Count(footer, "\n") + 1
	available := max(m.height-headerLines-footerLines-2*footerGap, 1)
	lines := strings.Split(body, "\n")
	if len(lines) <= available {
		return body
	}
	trimmed := lines[:available-1]
	return strings.Join(trimmed, "\n") + "\n" + mutedStyle.Render("↓ content truncated")
}

func (m Model) renderError() string {
	var b strings.Builder
	b.WriteString(errStyle.Render("Could not score " + m.owner + "/" + m.repo))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(max(m.width, 20)).Render(m.err.Error()))
	if isRateLimit(m.err) {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render(
			"Tip: set a GITHUB_TOKEN to lift the limit to 5,000 requests/hour."))
	}

	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Press r to retry."))
	return b.String()
}

func (m Model) renderActiveView() string {
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

		hint = "esc collapse · ←→ switch view · r refresh · q quit"
	case m.canSelect():
		hint = "↑↓ select · enter drill · ←→ switch view · r refresh · ? help · q quit"
	default:
		hint = "←→ switch view · r refresh · ? help · q quit"
	}
	keys := mutedStyle.Render(hint)
	// One line when it fits; otherwise tabs above hints, each clipped to width.
	if lipgloss.Width(tabs)+4+lipgloss.Width(keys) <= m.width {
		return tabs + "    " + keys
	}
	return ansi.Truncate(tabs, m.width, "…") + "\n" + ansi.Truncate(keys, m.width, "…")
}

func isRateLimit(err error) bool {
	var rl *github.RateLimitError
	return errors.As(err, &rl)
}
