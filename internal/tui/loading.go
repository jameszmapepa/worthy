package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/metrics"
)

type stageStatus struct {
	name    string
	state   metrics.StageState
	attempt int
}

var stageLabels = map[string]string{
	metrics.StageRepository:     "Repository",
	metrics.StageCommunity:      "Community profile",
	metrics.StageContributors:   "Contributor stats",
	metrics.StageCommits:        "Commit activity",
	metrics.StageReleases:       "Releases",
	metrics.StageWorkflows:      "CI workflows",
	metrics.StageClosedPulls:    "Closed pull requests",
	metrics.StageOpenPulls:      "Open pull requests",
	metrics.StageIssueTTFR:      "Issue response time",
	metrics.StagePRCohort:       "Recent PR cohort",
	metrics.StageNewcomerLabels: "Newcomer labels",
}

func newStages() []stageStatus {
	out := make([]stageStatus, len(metrics.Stages))
	for i, name := range metrics.Stages {
		out[i] = stageStatus{name: name, state: metrics.StagePending}
	}
	return out
}

func (m *Model) applyProgress(p metrics.Progress) {
	for i := range m.stages {
		if m.stages[i].name == p.Stage {
			m.stages[i].state = p.State
			m.stages[i].attempt = p.Attempt
		}
	}
	if p.Repo != nil {
		m.raw = *p.Repo
		m.hasRepo = true
	}
}

// stagesFinished counts stages that reached a terminal state.
func (m Model) stagesFinished() int {
	n := 0
	for _, s := range m.stages {
		switch s.state {
		case metrics.StageDone, metrics.StageDegraded, metrics.StageFailed:
			n++
		}
	}
	return n
}

func (m Model) renderLoading() string {
	elapsed := time.Since(m.loadStart).Round(time.Second)
	var b strings.Builder
	fmt.Fprintf(&b, "%s Scoring %s/%s … %s", m.spinner.View(), m.owner, m.repo,
		mutedStyle.Render(fmt.Sprintf("(%s · %d/%d)", elapsed, m.stagesFinished(), len(m.stages))))
	b.WriteString("\n\n")
	for _, s := range m.stages {
		b.WriteString("  ")
		b.WriteString(m.renderStage(s))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderStage(s stageStatus) string {
	label := stageLabels[s.name]
	switch s.state {
	case metrics.StageRunning:
		return m.spinner.View() + " " + labelStyle.Render(label)
	case metrics.StageRetrying:
		note := fmt.Sprintf("GitHub is computing stats, retry %d", s.attempt)
		return m.spinner.View() + " " + labelStyle.Render(label) + "  " + mutedStyle.Render(note)
	case metrics.StageDone:
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓") + " " + labelStyle.Render(label)
	case metrics.StageDegraded:
		return lipgloss.NewStyle().Foreground(colorAmber).Render(glyphWarn) + " " + labelStyle.Render(label) +
			"  " + mutedStyle.Render("unavailable, scored as neutral")
	case metrics.StageFailed:
		return lipgloss.NewStyle().Foreground(colorRed).Render(glyphCritical) + " " + labelStyle.Render(label)
	default:
		return mutedStyle.Render("· " + label)
	}
}
