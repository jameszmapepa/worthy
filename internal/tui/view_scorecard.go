package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// scorecardLabelWidth is the fixed column width for sub-score labels.
const scorecardLabelWidth = 22

// renderScorecard renders View 1: the headline composite + grade, per-indicator
// bars grouped by category, and the gate list.
func renderScorecard(r score.Report, width int) string {
	var b strings.Builder

	b.WriteString(renderHeadline(r))
	b.WriteString("\n\n")

	barWidth := clampWidth(width-scorecardLabelWidth-28, 10, 40)
	for _, cat := range r.Categories {
		b.WriteString(titleStyle.Render(fmt.Sprintf("%s  (%.0f%%)", cat.Label, cat.Value)))
		b.WriteString("\n")
		for _, s := range cat.Subs {
			b.WriteString(renderSubLine(s, barWidth))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(renderGates(r.Gates))
	return b.String()
}

// renderHeadline shows the big adjusted composite and the letter grade.
func renderHeadline(r score.Report) string {
	composite := lipgloss.NewStyle().Foreground(barColor(r.AdjustedComposite)).Bold(true).
		Render(fmt.Sprintf("%.1f", r.AdjustedComposite))
	grade := gradeStyle.Render("Grade " + r.Grade)
	return fmt.Sprintf("%s / 100   %s", composite, grade)
}

// renderSubLine renders one indicator: label, colored bar, value, raw metric.
func renderSubLine(s score.SubScore, barWidth int) string {
	label := labelStyle.Width(scorecardLabelWidth).Render(truncate(s.Label, scorecardLabelWidth))
	bar := renderBar(s.Value, barWidth)
	value := lipgloss.NewStyle().Foreground(barColor(s.Value)).
		Render(fmt.Sprintf("%5.1f", s.Value))
	raw := mutedStyle.Render(s.Raw)
	return fmt.Sprintf("%s %s %s  %s", label, bar, value, raw)
}

// renderGates lists the gates with severity glyphs and details.
func renderGates(gates []score.Gate) string {
	if len(gates) == 0 {
		return mutedStyle.Render("No gates triggered.")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Gates"))
	b.WriteString("\n")
	for _, g := range gates {
		glyph, c := severityGlyph(g.Severity)
		head := lipgloss.NewStyle().Foreground(c).Render(glyph + " " + g.Title)
		b.WriteString(head)
		if g.CapTo != nil {
			b.WriteString(mutedStyle.Render(fmt.Sprintf(" (caps at %.0f)", *g.CapTo)))
		}
		b.WriteString("\n  ")
		b.WriteString(mutedStyle.Render(g.Detail))
		b.WriteString("\n")
	}
	return b.String()
}

// truncate shortens s to at most n runes, adding an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// clampWidth constrains v to [lo, hi].
func clampWidth(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
