package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

const scorecardLabelWidth = 22

const scorecardBarWidthOverhead = 44

var panelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorBorder).
	Padding(0, 1)

var heroStyle = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(colorAccent).
	Padding(0, 2)

var (
	selectedMarkerStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedLabelStyle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

var detailStyle = lipgloss.NewStyle().
	MarginLeft(2).
	Border(lipgloss.NormalBorder(), false, false, false, true).
	BorderForeground(colorAccent).
	PaddingLeft(1)

func renderScorecard(r score.Report, width, selected int, expanded bool) string {
	var b strings.Builder

	b.WriteString(renderQuestionCards(r, width))
	b.WriteString("\n\n")

	b.WriteString(renderHero(r, width))
	b.WriteString("\n\n")

	barWidth := clampWidth(width-scorecardLabelWidth-scorecardBarWidthOverhead, 10, 28)
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

func renderQuestionCards(r score.Report, width int) string {
	halfW := max((width-6)/2, 22)

	card1 := renderQuestionCard(r.Maintained, halfW)
	card2 := renderQuestionCard(r.Contributable, halfW)

	var cards string
	if width < narrowTerminalWidth {
		cards = lipgloss.JoinVertical(lipgloss.Left, card1, card2)
	} else {
		cards = lipgloss.JoinHorizontal(lipgloss.Top, card1, "  ", card2)
	}

	caveat := confidenceCaveat(r.Confidence)
	if caveat == "" {
		return cards
	}
	return lipgloss.JoinVertical(lipgloss.Left, cards, caveat)
}

func renderQuestionCard(qs score.QuestionScore, width int) string {
	innerW := max(width-6, 10)

	question := titleStyle.Render(truncate(qs.Label, innerW))
	bigGrade := lipgloss.NewStyle().
		Foreground(barColor(qs.Value)).
		Bold(true).
		Render(qs.Grade)
	val := lipgloss.NewStyle().
		Foreground(barColor(qs.Value)).
		Render(fmt.Sprintf("  %.1f / 100", qs.Value))
	headline := bigGrade + val

	body := question + "\n" + headline
	if qs.Message != "" {
		body += "\n" + mutedStyle.Render(truncate(qs.Message, innerW))
	}

	return questionCardStyle.Width(width - 2).Render(body)
}

func confidenceCaveat(c score.ConfidenceLevel) string {
	switch c {
	case score.ConfidenceLow:
		return mutedStyle.Render(glyphWarn + " Limited data — scores may be imprecise")
	case score.ConfidenceMedium:
		return mutedStyle.Render(glyphInfo + " Some data unavailable — scores are broadly indicative")
	default:
		return ""
	}
}

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

func renderCategoryPanel(cat score.CategoryScore, barWidth, width, base, selected int, expanded bool) string {
	boxW := clampWidth(width-2, 30, 200)
	textW := boxW - 2

	rawBudget := max(textW-(scorecardLabelWidth+1+barWidth+1+5+1+2)-1, 6)

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
			b.WriteString(renderDetail(s, cat))
		}
		if i < len(cat.Subs)-1 {
			b.WriteString("\n")
		}
	}
	return panelStyle.Width(boxW).Render(b.String())
}

const subLabelWidth = scorecardLabelWidth - 2

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

	grade := mutedStyle.Render(score.LetterGrade(s.Value))
	raw := mutedStyle.Render(truncate(s.Raw, rawBudget))
	return fmt.Sprintf("%s%s %s %s%s  %s", marker, label, bar, value, grade, raw)
}

func renderDetail(s score.SubScore, cat score.CategoryScore) string {
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
