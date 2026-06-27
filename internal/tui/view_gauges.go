package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

const gaugeLabelWidth = 12

const narrowTerminalWidth = 70

const gaugeDetailBarWidthOverhead = 28

func renderGauges(r score.Report, raw score.RawMetrics, width, selected int, expanded bool) string {
	grade := lipgloss.NewStyle().Foreground(barColor(r.AdjustedComposite)).Bold(true).
		Render(fmt.Sprintf("  %s  ", r.Grade))
	head := titleStyle.Render("Overall grade ") + grade +
		mutedStyle.Render(fmt.Sprintf("  %.1f / 100", r.AdjustedComposite))

	gaugeWidth := clampWidth(width/2-gaugeLabelWidth-10, 12, 40)
	var gb strings.Builder
	for ci, cat := range r.Categories {
		gb.WriteString(renderGauge(cat.Label, cat.Value, gaugeWidth, ci == selected))
		gb.WriteString("\n")
	}
	gb.WriteString(renderGauge("Composite", r.AdjustedComposite, gaugeWidth, false))
	gaugePanel := titledPanel("Category gauges", strings.TrimRight(gb.String(), "\n"), colorBorder)

	sparkWidth := clampWidth(width/2-8, 16, 60)
	trend := titleStyle.Render("52-week commit trend") + "\n" +
		renderSparkline(raw.CommitsLast52Weeks, sparkWidth) + "\n\n" +
		headlineStats(raw)
	trendPanel := titledPanel("Activity", trend, colorBorder)

	var dashboard string
	if width < narrowTerminalWidth {
		dashboard = lipgloss.JoinVertical(lipgloss.Left, gaugePanel, trendPanel)
	} else {
		dashboard = lipgloss.JoinHorizontal(lipgloss.Top, gaugePanel, "  ", trendPanel)
	}
	out := head + "\n\n" + dashboard

	if expanded && selected >= 0 && selected < len(r.Categories) {
		out += "\n\n" + renderGaugeDetail(r.Categories[selected], width)
	}
	return out
}

func headlineStats(raw score.RawMetrics) string {
	rows := [][2]string{
		{"Stars", humanizeCount(raw.Stars)},
		{"Forks", humanizeCount(raw.Forks)},
		{"Watchers", humanizeCount(raw.Watchers)},
		{"Contributors", fmt.Sprintf("%d", raw.ContributorCount)},
		{"Releases", fmt.Sprintf("%d", raw.ReleaseCount)},
	}
	var b strings.Builder
	for i, row := range rows {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("%-13s", row[0])))
		b.WriteString(labelStyle.Render(row[1]))
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderGauge(label string, value float64, barWidth int, selected bool) string {
	bar := renderBar(value, barWidth)

	grade := mutedStyle.Render(score.LetterGrade(value))
	name := fmt.Sprintf("%-*s", gaugeLabelWidth, truncate(label, gaugeLabelWidth))
	marker := "  "
	if selected {
		marker = selectedMarkerStyle.Render("▸ ")
		name = selectedLabelStyle.Render(name)
	}
	return fmt.Sprintf("%s%s %s %5.1f%s", marker, name, bar, value, grade)
}

func renderGaugeDetail(cat score.CategoryScore, width int) string {
	barWidth := clampWidth(width/2-gaugeDetailBarWidthOverhead, 8, 24)
	lines := make([]string, 0, 1+len(cat.Subs))
	lines = append(lines, titleStyle.Render(cat.Label+" indicators"))
	for _, s := range cat.Subs {
		name := labelStyle.Render(fmt.Sprintf("%-22s", truncate(s.Label, 22)))
		bar := renderBar(s.Value, barWidth)
		val := lipgloss.NewStyle().Foreground(barColor(s.Value)).
			Render(fmt.Sprintf("%5.1f", s.Value))

		grade := mutedStyle.Render(score.LetterGrade(s.Value))
		raw := mutedStyle.Render(truncate(s.Raw, 28))
		lines = append(lines, fmt.Sprintf("%s %s %s%s  %s", name, bar, val, grade, raw))
	}
	return detailStyle.Render(strings.Join(lines, "\n"))
}

func renderSparkline(weekly []int, width int) string {
	if len(weekly) == 0 {
		return mutedStyle.Render("(no commit-activity data)")
	}
	return sparklineBlock(weekly, width)
}
