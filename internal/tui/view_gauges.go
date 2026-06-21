package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	"github.com/NimbleMarkets/ntcharts/v2/sparkline"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// gaugeLabelWidth is the fixed column width for gauge labels.
const gaugeLabelWidth = 12

// renderGauges renders View 3: static progress gauges for each category and the
// composite, plus an ntcharts sparkline of the commit trend.
func renderGauges(r score.Report, raw score.RawMetrics, width int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Category gauges"))
	b.WriteString("\n\n")

	gaugeWidth := clampWidth(width-gaugeLabelWidth-8, 12, 50)
	for _, cat := range r.Categories {
		b.WriteString(renderGauge(cat.Label, cat.Value, gaugeWidth))
		b.WriteString("\n")
	}
	b.WriteString(renderGauge("Composite", r.AdjustedComposite, gaugeWidth))
	b.WriteString("\n\n")

	b.WriteString(titleStyle.Render("Commit trend"))
	b.WriteString("\n")
	b.WriteString(renderSparkline(raw.CommitsLast52Weeks, clampWidth(width-2, 20, 78)))
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
