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
func renderRadar(r score.Report, width int) string {
	subs := flattenSubs(r)

	title := titleStyle.Render(fmt.Sprintf("Health radar — %d indicators", len(subs)))
	subtitle := mutedStyle.Render("rings at 25 / 50 / 75 / 100")
	head := title + "\n" + subtitle

	if len(subs) == 0 {
		return head + "\n\n" + mutedStyle.Render("(no indicators to plot)")
	}

	plot := plotRadar(subs)
	radarPanel := titledPanel("Radar", plot, colorBorder)

	listInner := radarIndicatorList(r) + "\n\n" + radarLegend()
	listPanel := titledPanel("Indicators", listInner, colorBorder)

	dashboard := lipgloss.JoinHorizontal(lipgloss.Top, radarPanel, "  ", listPanel)
	return head + "\n\n" + dashboard
}

// radarIndicatorList renders the sub-scores grouped by category, each line a
// colored dot + name + gradient-colored value, for the right-hand panel.
func radarIndicatorList(r score.Report) string {
	var b strings.Builder
	for ci, c := range r.Categories {
		dot := lipgloss.NewStyle().Foreground(categoryColor(c.Key)).Render("●")
		b.WriteString(fmt.Sprintf("%s %s\n", dot, titleStyle.Render(c.Label)))
		for _, s := range c.Subs {
			name := labelStyle.Render(fmt.Sprintf("  %-20s", truncate(s.Label, 20)))
			val := lipgloss.NewStyle().Foreground(gradientColor(s.Value)).
				Render(fmt.Sprintf("%5.1f", s.Value))
			b.WriteString(name + " " + val + "\n")
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

// plotRadar draws the radar onto a braille canvas and returns it as a string.
func plotRadar(subs []indicator) string {
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

	// Faint spokes to each axis tip.
	for i := range subs {
		tx, ty := project(100, i, n, cx, cy, radius)
		cv.line(int(cx), int(cy), int(tx), int(ty), grid)
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
	// Emphasize each vertex in its category color.
	for i, ind := range subs {
		cv.set(int(points[i][0]), int(points[i][1]), ind.color)
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
