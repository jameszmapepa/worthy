package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jameszmapepa/worthy/internal/github"
)

const footerGap = 2

const selectionMarker = "▸"

const detailBlockLines = 7

func (m Model) render() string {
	header := m.renderHeader()
	footer := m.renderFooter()
	if m.height <= 0 {
		return header + "\n\n" + m.renderBody() + "\n\n" + footer
	}
	m.syncViewport(false)
	return header + "\n\n" + m.viewport.View() + "\n\n" + footer
}

func (m Model) renderHeader() string {
	grade := ""
	if m.state == stateLoaded {
		grade = m.report.Grade
	}
	return renderHeaderPanel(
		m.owner, m.repo, m.raw,
		m.state == stateLoaded || m.hasRepo, m.client.Authenticated(), m.client.RateInfo(), m.width, grade, m.asciiIcons,
	)
}

func (m Model) renderBody() string {
	switch m.state {
	case stateLoading:
		return m.renderLoading()
	case stateErrored:
		return m.renderError()
	default:
		return m.renderActiveView()
	}
}

func (m *Model) syncViewport(follow bool) {
	if m.height <= 0 {
		return
	}
	header := m.renderHeader()
	footer := m.renderFooter()
	avail := max(m.height-lines(header)-lines(footer)-2*footerGap, 1)
	body := m.renderBody()
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(avail)
	m.viewport.SetContent(body)
	if !follow {
		return
	}
	if m.selected == 0 && !m.expanded {
		m.viewport.GotoTop()
		return
	}
	sel := selectedLine(body)
	if sel < 0 {
		return
	}
	last := sel
	if m.expanded {
		last = min(sel+detailBlockLines, lines(body)-1)
	}
	m.ensureVisible(sel, last)
}

func (m *Model) ensureVisible(first, last int) {
	top := m.viewport.YOffset()
	h := m.viewport.Height()
	switch {
	case first < top:
		m.viewport.SetYOffset(first)
	case last >= top+h:
		m.viewport.SetYOffset(max(last-h+1, 0))
		if m.viewport.YOffset() > first {
			m.viewport.SetYOffset(first)
		}
	}
}

func selectedLine(body string) int {
	for i, l := range strings.Split(body, "\n") {
		if strings.Contains(l, selectionMarker) {
			return i
		}
	}
	return -1
}

func lines(s string) int { return strings.Count(s, "\n") + 1 }

func (m Model) renderError() string {
	var b strings.Builder
	b.WriteString(errStyle.Render("Could not score " + m.owner + "/" + m.repo))
	b.WriteString("\n\n")
	errText := "unknown error"
	if m.err != nil {
		errText = m.err.Error()
	}
	b.WriteString(lipgloss.NewStyle().Width(max(m.width, 20)).Render(errText))
	if isRateLimit(m.err) {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render(
			"Tip: set a GITHUB_TOKEN to lift the limit to 5,000 requests/hour.",
		))
	}

	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Press r to retry."))
	return b.String()
}

func (m Model) renderActiveView() string {
	if m.helpVisible {
		return m.renderHelp()
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

	var keys string
	if m.status != "" {
		keys = lipgloss.NewStyle().Foreground(colorGreen).Render("✓ " + m.status)
	} else {
		keys = helpModel(m.width).ShortHelpView(m.shortHelp())
		if m.height > 0 && m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
			keys += mutedStyle.Render(fmt.Sprintf(" · ↕ %.0f%%", m.viewport.ScrollPercent()*100))
		}
	}
	if lipgloss.Width(tabs)+4+lipgloss.Width(keys) <= m.width {
		return tabs + "    " + keys
	}
	return ansi.Truncate(tabs, m.width, "…") + "\n" + ansi.Truncate(keys, m.width, "…")
}

func isRateLimit(err error) bool {
	var rl *github.RateLimitError
	return errors.As(err, &rl)
}
