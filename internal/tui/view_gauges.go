package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// gaugeLabelWidth is the fixed column width for gauge labels.
const gaugeLabelWidth = 12

// narrowTerminalWidth is the column threshold below which the two-panel
// dashboard switches from JoinHorizontal to JoinVertical. Below this width
// the panels would be too cramped side-by-side to be readable.
const narrowTerminalWidth = 70

// gaugeDetailBarWidthOverhead is the total fixed column overhead in a gauge
// detail panel row (22-char label + spacing + value + border/padding). Used to
// derive bar width from half the terminal width.
const gaugeDetailBarWidthOverhead = 28

// renderGauges renders View 3 as a two-panel dashboard: category + composite
// gauges on the left, and the 52-week commit sparkline plus headline stats on
// the right. Below narrowTerminalWidth columns the panels are stacked
// vertically instead of placed side-by-side.
func renderGauges(r score.Report, raw score.RawMetrics, width, selected int, expanded bool) string {
	grade := lipgloss.NewStyle().Foreground(barColor(r.AdjustedComposite)).Bold(true).
		Render(fmt.Sprintf("  %s  ", r.Grade))
	head := titleStyle.Render("Overall grade ") + grade +
		mutedStyle.Render(fmt.Sprintf("  %.1f / 100", r.AdjustedComposite))

	// Left panel: gauges. selected indexes a category; the composite gauge is
	// never selectable.
	gaugeWidth := clampWidth(width/2-gaugeLabelWidth-10, 12, 40)
	var gb strings.Builder
	for ci, cat := range r.Categories {
		gb.WriteString(renderGauge(cat.Label, cat.Value, gaugeWidth, ci == selected))
		gb.WriteString("\n")
	}
	gb.WriteString(renderGauge("Composite", r.AdjustedComposite, gaugeWidth, false))
	gaugePanel := titledPanel("Category gauges", strings.TrimRight(gb.String(), "\n"), colorBorder)

	// Right panel: sparkline + headline stats.
	sparkWidth := clampWidth(width/2-8, 16, 60)
	trend := titleStyle.Render("52-week commit trend") + "\n" +
		renderSparkline(raw.CommitsLast52Weeks, sparkWidth) + "\n\n" +
		headlineStats(raw)
	trendPanel := titledPanel("Activity", trend, colorBorder)

	// C4: below the narrow threshold, stack the panels vertically so each can
	// use the full width rather than half. Above the threshold they sit
	// side-by-side with a two-space gap.
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

// headlineStats renders a few at-a-glance repository numbers for the right
// panel of the gauges view.
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

// renderGauge renders one progress bar for a 0..100 value using renderBar
// (C7 — avoids allocating a fresh progress.Model every render), with a text
// grade (C5 — meaning survives NO_COLOR / monochrome terminals).
func renderGauge(label string, value float64, barWidth int, selected bool) string {
	bar := renderBar(value, barWidth)
	// C5: text-tier grade so the quality level is legible without color.
	grade := mutedStyle.Render(score.LetterGrade(value))
	name := fmt.Sprintf("%-*s", gaugeLabelWidth, truncate(label, gaugeLabelWidth))
	marker := "  "
	if selected {
		marker = selectedMarkerStyle.Render("▸ ")
		name = selectedLabelStyle.Render(name)
	}
	return fmt.Sprintf("%s%s %s %5.1f%s", marker, name, bar, value, grade)
}

// renderGaugeDetail renders the inline drill-down panel for a selected category
// gauge: its constituent indicators as labeled bars, tracing the category score
// back to the sub-scores behind it. Mirrors the scorecard's detail placement.
func renderGaugeDetail(cat score.CategoryScore, width int) string {
	barWidth := clampWidth(width/2-gaugeDetailBarWidthOverhead, 8, 24)
	lines := []string{titleStyle.Render(cat.Label + " indicators")}
	for _, s := range cat.Subs {
		name := labelStyle.Render(fmt.Sprintf("%-22s", truncate(s.Label, 22)))
		bar := renderBar(s.Value, barWidth)
		val := lipgloss.NewStyle().Foreground(barColor(s.Value)).
			Render(fmt.Sprintf("%5.1f", s.Value))
		// C5: text-tier grade alongside the bar value.
		grade := mutedStyle.Render(score.LetterGrade(s.Value))
		raw := mutedStyle.Render(truncate(s.Raw, 28))
		lines = append(lines, fmt.Sprintf("%s %s %s%s  %s", name, bar, val, grade, raw))
	}
	return detailStyle.Render(strings.Join(lines, "\n"))
}

// renderSparkline renders the commit-count series as a hand-rolled Unicode
// block sparkline (▁▂▃▄▅▆▇█). This replaces the ntcharts dependency, which
// dragged in image/png and golang.org/x/image for a single-row sparkline.
// sparklineBlock handles the normalization and sampling; renderSparkline adds
// the empty-data guard and muted ANSI styling.
func renderSparkline(weekly []int, width int) string {
	if len(weekly) == 0 {
		return mutedStyle.Render("(no commit-activity data)")
	}
	return sparklineBlock(weekly, width)
}
