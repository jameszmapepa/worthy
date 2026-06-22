package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/sparkline"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// gaugeLabelWidth is the fixed column width for gauge labels.
const gaugeLabelWidth = 12

// renderGauges renders View 3 as a two-panel dashboard: category + composite
// gauges on the left, and the 52-week commit sparkline plus headline stats on
// the right, joined horizontally.
func renderGauges(r score.Report, raw score.RawMetrics, width int) string {
	grade := lipgloss.NewStyle().Foreground(barColor(r.AdjustedComposite)).Bold(true).
		Render(fmt.Sprintf("  %s  ", r.Grade))
	head := titleStyle.Render("Overall grade ") + grade +
		mutedStyle.Render(fmt.Sprintf("  %.1f / 100", r.AdjustedComposite))

	// Left panel: gauges.
	gaugeWidth := clampWidth(width/2-gaugeLabelWidth-10, 12, 40)
	var gb strings.Builder
	for _, cat := range r.Categories {
		gb.WriteString(renderGauge(cat.Label, cat.Value, gaugeWidth))
		gb.WriteString("\n")
	}
	gb.WriteString(renderGauge("Composite", r.AdjustedComposite, gaugeWidth))
	gaugePanel := titledPanel("Category gauges", strings.TrimRight(gb.String(), "\n"), colorBorder)

	// Right panel: sparkline + headline stats.
	sparkWidth := clampWidth(width/2-8, 16, 60)
	trend := titleStyle.Render("52-week commit trend") + "\n" +
		renderSparkline(raw.CommitsLast52Weeks, sparkWidth) + "\n\n" +
		headlineStats(raw)
	trendPanel := titledPanel("Activity", trend, colorBorder)

	dashboard := lipgloss.JoinHorizontal(lipgloss.Top, gaugePanel, "  ", trendPanel)
	return head + "\n\n" + dashboard
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

// renderGauge renders one static progress bar for a 0..100 value. The bar uses
// the value's band color (green/amber/red) and is rendered with ViewAs so it is
// a fixed snapshot rather than an animation.
func renderGauge(label string, value float64, barWidth int) string {
	c := barColor(value)
	prog := progress.New(progress.WithColors(c, c), progress.WithWidth(barWidth))
	label = fmt.Sprintf("%-*s", gaugeLabelWidth, truncate(label, gaugeLabelWidth))
	return fmt.Sprintf("%s %s %5.1f", label, prog.ViewAs(value/100), value)
}

// renderSparkline renders the commit-count series as an ntcharts sparkline.
func renderSparkline(weekly []int, width int) string {
	if len(weekly) == 0 {
		return mutedStyle.Render("(no commit-activity data)")
	}
	data := make([]float64, len(weekly))
	for i, v := range weekly {
		data[i] = float64(v)
	}
	sl := sparkline.New(width, 1)
	sl.PushAll(data)
	sl.Draw()
	return sl.View()
}
