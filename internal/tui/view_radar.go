package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// Radar canvas geometry, in character cells. The braille surface gives 2x4
// sub-dot resolution on top of this, and braille sub-dots are ~square, so the
// projection needs no extra terminal aspect fudge — that is the whole reason we
// render onto braille rather than single runes.
const (
	radarCols     = 38 // canvas width in cells (fits inside 80 cols with labels)
	radarRows     = 18 // canvas height in cells
	radarRingsNum = 4  // concentric rings (25 / 50 / 75 / 100)
)

// project maps a sub-score value (0..100) on axis index of n axes to a pixel
// coordinate around center (cx,cy) with the given pixel radius. Axis 0 points
// straight up; axes proceed clockwise. Screen y grows downward, so "up" is a
// smaller y.
func project(value float64, index, n int, cx, cy, radius float64) (x, y float64) {
	if n < 1 {
		n = 1
	}
	angle := -math.Pi/2 + 2*math.Pi*float64(index)/float64(n)
	r := value / 100 * radius
	return cx + r*math.Cos(angle), cy + r*math.Sin(angle)
}

// renderRadar renders View 2 as a two-panel dashboard: the braille radar in a
// bordered panel on the left, and a per-category indicator list on the right,
// joined horizontally so the previously empty space is used.
func renderRadar(r score.Report, width, selected int, expanded bool) string {
	subs := flattenSubs(r)

	title := titleStyle.Render(fmt.Sprintf("Health radar — %d indicators", len(subs)))
	subtitle := mutedStyle.Render("rings at 25 / 50 / 75 / 100")
	head := title + "\n" + subtitle

	if len(subs) == 0 {
		return head + "\n\n" + mutedStyle.Render("(no indicators to plot)")
	}

	plot := plotRadar(subs, selected)
	radarPanel := titledPanel("Radar", plot, colorBorder)

	listInner := radarIndicatorList(r, selected) + "\n\n" + radarLegend()
	listPanel := titledPanel("Indicators", listInner, colorBorder)

	dashboard := lipgloss.JoinHorizontal(lipgloss.Top, radarPanel, "  ", listPanel)
	out := head + "\n\n" + dashboard

	if expanded {
		if cat, sub, ok := indicatorAt(r, selected); ok {
			out += "\n\n" + renderDetail(sub, cat, width)
		}
	}
	return out
}

// radarIndicatorList renders the sub-scores grouped by category, each line a
// colored dot + name + gradient-colored value, for the right-hand panel. The
// indicator at the flattened index selected is marked; pass -1 for none.
func radarIndicatorList(r score.Report, selected int) string {
	var b strings.Builder
	idx := 0
	for ci, c := range r.Categories {
		dot := lipgloss.NewStyle().Foreground(categoryColor(c.Key)).Render("●")
		b.WriteString(fmt.Sprintf("%s %s\n", dot, titleStyle.Render(c.Label)))
		for _, s := range c.Subs {
			label := truncate(s.Label, 20)
			marker := "  "
			name := labelStyle.Render(fmt.Sprintf("%-20s", label))
			if idx == selected {
				marker = selectedMarkerStyle.Render("▸ ")
				name = selectedLabelStyle.Render(fmt.Sprintf("%-20s", label))
			}
			val := lipgloss.NewStyle().Foreground(gradientColor(s.Value)).
				Render(fmt.Sprintf("%5.1f", s.Value))
			b.WriteString(marker + name + " " + val + "\n")
			idx++
		}
		if ci < len(r.Categories)-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// indicator is one sub-score paired with the color of its category.
type indicator struct {
	sub   score.SubScore
	color color.Color
}

// flattenSubs flattens all category sub-scores in display order, tagging each
// with its category color.
func flattenSubs(r score.Report) []indicator {
	var out []indicator
	for _, c := range r.Categories {
		col := categoryColor(c.Key)
		for _, s := range c.Subs {
			out = append(out, indicator{sub: s, color: col})
		}
	}
	return out
}

// indicatorAt resolves a flattened indicator index to its sub-score and the
// category it belongs to, in the same display order as flattenSubs and the
// scorecard. ok is false when idx is out of range. Reused by the radar
// drill-down to surface the same detail panel as the scorecard.
func indicatorAt(r score.Report, idx int) (score.CategoryScore, score.SubScore, bool) {
	if idx < 0 {
		return score.CategoryScore{}, score.SubScore{}, false
	}
	i := 0
	for _, c := range r.Categories {
		for _, s := range c.Subs {
			if i == idx {
				return c, s, true
			}
			i++
		}
	}
	return score.CategoryScore{}, score.SubScore{}, false
}

// plotRadar draws the radar onto a braille canvas and returns it as a string.
// The axis at the flattened index selected is lit bright to tie the plot to the
// indicator list; pass -1 for no selection.
func plotRadar(subs []indicator, selected int) string {
	cv := newBrailleCanvas(radarCols, radarRows)
	cx := float64(cv.pxW) / 2
	cy := float64(cv.pxH) / 2
	radius := math.Min(cx, cy) - 1
	n := len(subs)

	// Darkened gridlines so the bright score polygon pops against them.
	grid := lipgloss.Darken(colorTrackEmpty, 0.35)

	// Faint concentric rings at 25/50/75/100.
	for ring := 1; ring <= radarRingsNum; ring++ {
		drawRing(cv, cx, cy, radius*float64(ring)/float64(radarRingsNum), grid)
	}

	// Faint spokes to each axis tip; the selected axis lit bright.
	for i := range subs {
		tx, ty := project(100, i, n, cx, cy, radius)
		col := grid
		if i == selected {
			col = colorFg
		}
		cv.line(int(cx), int(cy), int(tx), int(ty), col)
	}

	// Bright score polygon: connect consecutive points, closed.
	points := make([][2]float64, n)
	for i, ind := range subs {
		px, py := project(ind.sub.Value, i, n, cx, cy, radius)
		points[i] = [2]float64{px, py}
	}
	for i := range points {
		a := points[i]
		bpt := points[(i+1)%n]
		cv.line(int(a[0]), int(a[1]), int(bpt[0]), int(bpt[1]), colorAccent)
	}
	// Emphasize each vertex in its category color; the selected one bright.
	for i, ind := range subs {
		col := ind.color
		if i == selected {
			col = colorFg
		}
		cv.set(int(points[i][0]), int(points[i][1]), col)
	}

	return cv.render()
}

// drawRing approximates a circle of the given pixel radius by stepping the
// angle and lighting each point.
func drawRing(cv *brailleCanvas, cx, cy, radius float64, col color.Color) {
	if radius < 1 {
		return
	}
	steps := int(2 * math.Pi * radius)
	if steps < 16 {
		steps = 16
	}
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		cv.set(int(cx+radius*math.Cos(a)), int(cy+radius*math.Sin(a)), col)
	}
}

// radarLegend shows the category color key.
func radarLegend() string {
	type entry struct{ key, name string }
	entries := []entry{
		{score.CategoryActivity, "Activity"},
		{score.CategoryCommunity, "Community"},
		{score.CategorySecurity, "Security"},
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		dot := lipgloss.NewStyle().Foreground(categoryColor(e.key)).Render("●")
		parts[i] = dot + " " + e.name
	}
	return mutedStyle.Render("Legend: ") + strings.Join(parts, "   ")
}
