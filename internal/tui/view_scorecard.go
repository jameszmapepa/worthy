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

// renderScorecard renders View 1: a hero block (composite, grade, verdict),
// per-category bordered panels of indicator bars, and gate badges.
func renderScorecard(r score.Report, width int) string {
	var b strings.Builder

	b.WriteString(renderHero(r, width))
	b.WriteString("\n\n")

	barWidth := clampWidth(width-scorecardLabelWidth-44, 10, 28)
	for _, cat := range r.Categories {
		b.WriteString(renderCategoryPanel(cat, barWidth, width))
		b.WriteString("\n")
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
func renderCategoryPanel(cat score.CategoryScore, barWidth, width int) string {
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
		b.WriteString(renderSubLine(s, barWidth, rawBudget))
		if i < len(cat.Subs)-1 {
			b.WriteString("\n")
		}
	}
	return panelStyle.Width(boxW).Render(b.String())
}

// renderSubLine renders one indicator on a single line: label, colored bar,
// value, and the (truncated) raw metric so the row never wraps.
func renderSubLine(s score.SubScore, barWidth, rawBudget int) string {
	label := labelStyle.Width(scorecardLabelWidth).Render(truncate(s.Label, scorecardLabelWidth))
	bar := renderBar(s.Value, barWidth)
	value := lipgloss.NewStyle().Foreground(barColor(s.Value)).
		Render(fmt.Sprintf("%5.1f", s.Value))
	raw := mutedStyle.Render(truncate(s.Raw, rawBudget))
	return fmt.Sprintf("%s %s %s  %s", label, bar, value, raw)
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
