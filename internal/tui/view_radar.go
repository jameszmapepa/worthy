package tui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// radarRadius is the plot radius in character cells; the grid is roughly
// 2*radius+1 wide. Kept small enough to stay readable at 80 columns.
const radarRadius = 9

// renderRadar renders View 2: a hand-rolled ASCII radar plotting each
// category's score as a point on its own axis, joined into a polygon, with a
// legend below.
func renderRadar(r score.Report, width int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Category radar"))
	b.WriteString("\n\n")
	b.WriteString(plotRadar(r.Categories))
	b.WriteString("\n")
	b.WriteString(radarLegend(r.Categories))
	return b.String()
}

// plotRadar draws the radar grid. Each category gets an evenly spaced axis from
// the center; its value (0..100) sets the radius of its plotted point.
func plotRadar(cats []score.CategoryScore) string {
	const size = 2*radarRadius + 1
	grid := make([][]rune, size)
	for y := range grid {
		grid[y] = make([]rune, size)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}
	cx, cy := radarRadius, radarRadius
	grid[cy][cx] = '+'

	n := len(cats)
	for i, cat := range cats {
		angle := -math.Pi/2 + 2*math.Pi*float64(i)/float64(maxInt(n, 1))
		dist := cat.Value / 100 * float64(radarRadius)
		// Plot a few points along the spoke so axes are visible.
		for step := 1.0; step <= dist; step++ {
			x := cx + int(math.Round(math.Cos(angle)*step))
			y := cy + int(math.Round(math.Sin(angle)*step*0.5)) // squash for cell aspect
			if inGrid(x, y, size) {
				grid[y][x] = '·'
			}
		}
		x := cx + int(math.Round(math.Cos(angle)*dist))
		y := cy + int(math.Round(math.Sin(angle)*dist*0.5))
		if inGrid(x, y, size) {
			grid[y][x] = '●'
		}
	}

	var b strings.Builder
	for _, row := range grid {
		b.WriteString(strings.TrimRight(string(row), " "))
		b.WriteString("\n")
	}
	return b.String()
}

// radarLegend lists the axes with their values and band colors.
func radarLegend(cats []score.CategoryScore) string {
	var parts []string
	for _, cat := range cats {
		dot := lipgloss.NewStyle().Foreground(barColor(cat.Value)).Render("●")
		parts = append(parts, fmt.Sprintf("%s %s %.0f%%", dot, cat.Label, cat.Value))
	}
	return strings.Join(parts, "    ")
}

func inGrid(x, y, size int) bool {
	return x >= 0 && x < size && y >= 0 && y < size
}
