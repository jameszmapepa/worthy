package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// scorecardLabelWidth is the fixed column width for sub-score labels.
const scorecardLabelWidth = 22

// panelStyle wraps a category in a subtle titled border.
var panelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorBorder).
	Padding(0, 1)

// heroStyle frames the headline composite + grade + verdict.
var heroStyle = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(colorAccent).
	Padding(0, 2)

// selectedMarkerStyle and selectedLabelStyle highlight the focused indicator.
var (
	selectedMarkerStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedLabelStyle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// detailStyle frames the inline drill-down panel below the selected indicator
// with a left accent rule.
var detailStyle = lipgloss.NewStyle().
	MarginLeft(2).
	Border(lipgloss.NormalBorder(), false, false, false, true).
	BorderForeground(colorAccent).
	PaddingLeft(1)

// renderScorecard renders View 1: a hero block (composite, grade, verdict),
// per-category bordered panels of indicator bars, and gate badges. When
// selected >= 0 the indicator at that flattened index is highlighted; when
// expanded is also true, an inline detail panel is rendered below it.
func renderScorecard(r score.Report, width, selected int, expanded bool) string {
	var b strings.Builder

	b.WriteString(renderHero(r, width))
	b.WriteString("\n\n")

	barWidth := clampWidth(width-scorecardLabelWidth-44, 10, 28)
	base := 0
	for _, cat := range r.Categories {
		b.WriteString(renderCategoryPanel(cat, barWidth, width, base, selected, expanded))
		b.WriteString("\n")
		base += len(cat.Subs)
	}

	b.WriteString("\n")
	b.WriteString(renderGates(r.Gates))
	return b.String()
}

// renderHero frames the big composite, grade, and one-line verdict.
func renderHero(r score.Report, width int) string {
	big := lipgloss.NewStyle().Foreground(barColor(r.AdjustedComposite)).Bold(true).
		Render(fmt.Sprintf("%.1f", r.AdjustedComposite))
	grade := gradeStyle.Render("Grade " + r.Grade)
	headline := fmt.Sprintf("%s / 100   %s", big, grade)

	body := headline
	if r.Verdict != "" {
		body += "\n" + labelStyle.Render(truncate(r.Verdict, clampWidth(width-8, 30, 120)))
	}
	return heroStyle.Render(body)
}

// renderCategoryPanel renders one category's indicators inside a titled panel.
// base is the flattened index of this category's first sub-score; selected is
// the globally-selected index (or <0 for none) and expanded toggles the inline
// detail panel below the selected row.
func renderCategoryPanel(cat score.CategoryScore, barWidth, width, base, selected int, expanded bool) string {
	// lipgloss Width(boxW) includes padding; border is 2 cells outside it. So
	// box = width-2 and usable text = box-2.
	boxW := clampWidth(width-2, 30, 200)
	textW := boxW - 2
	// Fixed prefix per row: label + space + bar + space + value(5) + two spaces.
	// One extra cell of slack keeps a full-width row from wrapping its tail.
	rawBudget := textW - (scorecardLabelWidth + 1 + barWidth + 1 + 5 + 2) - 1
	if rawBudget < 6 {
		rawBudget = 6
	}

	var b strings.Builder
	dot := lipgloss.NewStyle().Foreground(categoryColor(cat.Key)).Render("●")
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s %s", dot, cat.Label)))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  %.0f%% · weight %.0f%%", cat.Value, cat.Weight*100)))
	b.WriteString("\n")
	for i, s := range cat.Subs {
		sel := base+i == selected
		b.WriteString(renderSubLine(s, barWidth, rawBudget, sel))
		if sel && expanded {
			b.WriteString("\n")
			b.WriteString(renderDetail(s, cat, textW))
		}
		if i < len(cat.Subs)-1 {
			b.WriteString("\n")
		}
	}
	return panelStyle.Width(boxW).Render(b.String())
}

// subLabelWidth reserves two cells for the selection marker so selected and
// unselected rows stay column-aligned.
const subLabelWidth = scorecardLabelWidth - 2

// renderSubLine renders one indicator on a single line: a selection marker,
// label, colored bar, value, and the (truncated) raw metric so the row never
// wraps. The selected row is highlighted.
func renderSubLine(s score.SubScore, barWidth, rawBudget int, sel bool) string {
	text := truncate(s.Label, subLabelWidth)
	marker := "  "
	label := labelStyle.Width(subLabelWidth).Render(text)
	if sel {
		marker = selectedMarkerStyle.Render("▸ ")
		label = selectedLabelStyle.Width(subLabelWidth).Render(text)
	}
	bar := renderBar(s.Value, barWidth)
	value := lipgloss.NewStyle().Foreground(barColor(s.Value)).
		Render(fmt.Sprintf("%5.1f", s.Value))
	raw := mutedStyle.Render(truncate(s.Raw, rawBudget))
	return fmt.Sprintf("%s%s %s %s  %s", marker, label, bar, value, raw)
}

// renderDetail renders the inline drill-down panel for the selected indicator:
// formula, raw metric, weight, weighted contribution to its category, and any
// gates whose condition references it.
func renderDetail(s score.SubScore, cat score.CategoryScore, width int) string {
	share := s.Weight * s.Value
	pct := 0.0
	if cat.Value > 0 {
		pct = share / cat.Value * 100
	}
	gates := "none"
	if len(s.Gates) > 0 {
		gates = strings.Join(s.Gates, ", ")
	}
	field := func(name, val string) string {
		return mutedStyle.Render(fmt.Sprintf("%-9s", name)) + val
	}
	lines := []string{
		field("Formula", s.Formula),
		field("Value", fmt.Sprintf("%.1f / 100", s.Value)),
		field("Raw", s.Raw),
		field("Weight", fmt.Sprintf("%.0f%% of %s", s.Weight*100, cat.Label)),
		field("Share", fmt.Sprintf("%.1f of %.1f category (%.0f%%)", share, cat.Value, pct)),
		field("Gates", gates),
	}
	return detailStyle.Render(strings.Join(lines, "\n"))
}

// renderGates renders triggered gates as colored severity badges with details.
func renderGates(gates []score.Gate) string {
	if len(gates) == 0 {
		return mutedStyle.Render("No gates triggered.")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Gates"))
	b.WriteString("\n")
	for _, g := range gates {
		b.WriteString(renderGateBadge(g))
		b.WriteString("  ")
		b.WriteString(mutedStyle.Render(g.Detail))
		b.WriteString("\n")
	}
	return b.String()
}

// renderGateBadge renders a single gate as a colored badge: glyph, title, and
// any composite cap it imposes.
func renderGateBadge(g score.Gate) string {
	glyph, c := severityGlyph(g.Severity)
	text := glyph + " " + g.Title
	if g.CapTo != nil {
		text += fmt.Sprintf(" · caps %.0f", *g.CapTo)
	}
	return lipgloss.NewStyle().
		Foreground(colorBackground).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render(text)
}
