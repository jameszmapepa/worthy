package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

func renderExplain(r score.Report, width int) string {
	var b strings.Builder

	verdict := r.Verdict
	if verdict == "" {
		verdict = fmt.Sprintf("Grade %s", r.Grade)
	}
	b.WriteString(heroStyle.Render(truncate(verdict, clampWidth(width-8, 30, 120))))
	b.WriteString("\n\n")

	strong, weak := score.Drivers(r)
	b.WriteString(titleStyle.Render("Strongest"))
	b.WriteString("\n")
	b.WriteString(renderDriverList(strong, "▲"))
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Weakest"))
	b.WriteString("\n")
	b.WriteString(renderDriverList(weak, "▼"))
	b.WriteString("\n\n")

	b.WriteString(titleStyle.Render("Gates"))
	b.WriteString("\n")
	b.WriteString(renderGateGuidance(r.Gates))

	return b.String()
}

func renderDriverList(subs []score.SubScore, arrow string) string {
	if len(subs) == 0 {
		return mutedStyle.Render("  (none)")
	}
	var b strings.Builder
	for i, s := range subs {
		mark := lipgloss.NewStyle().Foreground(barColor(s.Value)).Render(arrow)
		label := labelStyle.Width(scorecardLabelWidth).Render(truncate(s.Label, scorecardLabelWidth))
		val := lipgloss.NewStyle().Foreground(barColor(s.Value)).Render(fmt.Sprintf("%5.1f", s.Value))
		fmt.Fprintf(&b, "  %s %s %s", mark, label, val)
		if i < len(subs)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderGateGuidance(gates []score.Gate) string {
	if len(gates) == 0 {
		return lipgloss.NewStyle().Foreground(colorGreen).Render("✓ No gates triggered.")
	}
	var b strings.Builder
	for i, g := range gates {
		b.WriteString(renderGateBadge(g))
		b.WriteString("  ")
		b.WriteString(mutedStyle.Render(g.Detail))
		if g.HowToClear != "" {
			b.WriteString("\n    ")
			b.WriteString(labelStyle.Render("→ " + g.HowToClear))
		}
		if i < len(gates)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
