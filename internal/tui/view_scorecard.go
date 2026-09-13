package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

const scorecardLabelWidth = 22

const scorecardBarWidthOverhead = 44

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
	b.WriteString(renderGates(r.Gates, width))
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

// panelTextWidth is the content width inside a panel of outer width boxW:
// Style.Width counts the 2 border and 2 padding columns.
func panelTextWidth(boxW int) int { return boxW - 4 }

// subLineOverhead is every column of a sub-score line except the bar and the
// raw text: marker(2) label(22) gap bar gap value(5) grade(2) gap(2).
const subLineOverhead = 2 + scorecardLabelWidth + 1 + 1 + 5 + 2 + 2

// rawBudgetFor returns how many columns remain for the raw-metric text; below
// minRawBudget the column is dropped rather than squeezed.
func rawBudgetFor(textW, barWidth int) int {
	n := textW - subLineOverhead - barWidth
	if n < minRawBudget {
		return 0
	}
	return n
}

const minRawBudget = 6

func renderCategoryPanel(cat score.CategoryScore, barWidth, width, base, selected int, expanded bool) string {
	boxW := clampWidth(width-2, 30, 200)
	textW := panelTextWidth(boxW)
	rawBudget := rawBudgetFor(textW, barWidth)

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

const subLabelWidth = scorecardLabelWidth - 2

func renderSubLine(s score.SubScore, barWidth, rawBudget int, sel bool) string {
	text := truncate(s.Label, subLabelWidth)
	marker := "  "
	label := labelStyle.Width(subLabelWidth).Render(text)
	if sel {
		marker = selectedMarkerStyle.Render(selectionMarker + " ")
		label = selectedLabelStyle.Width(subLabelWidth).Render(text)
	}
	bar := renderBar(s.Value, barWidth)
	value := lipgloss.NewStyle().Foreground(barColor(s.Value)).
		Render(fmt.Sprintf("%5.1f", s.Value))

	grade := mutedStyle.Width(2).Render(score.LetterGrade(s.Value))
	line := fmt.Sprintf("%s%s %s %s%s", marker, label, bar, value, grade)
	if rawBudget > 0 {
		line += "  " + mutedStyle.Render(truncate(s.Raw, rawBudget))
	}
	return line
}

// detailIndent is detailStyle's margin, border and padding.
const detailIndent = 4

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
	return detailStyle.Width(max(width-detailIndent, 20)).Render(strings.Join(lines, "\n"))
}

func renderGates(gates []score.Gate, width int) string {
	if len(gates) == 0 {
		return mutedStyle.Render("No gates triggered.")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Gates"))
	b.WriteString("\n")
	for _, g := range gates {
		b.WriteString(renderGateLine(g, width))
		b.WriteString("\n")
	}
	return b.String()
}

// renderGateLine puts the badge beside its detail, wrapping the detail to the
// remaining width so long explanations never run past the terminal edge.
func renderGateLine(g score.Gate, width int) string {
	badge := renderGateBadge(g)
	detailW := width - lipgloss.Width(badge) - 2
	if detailW < 16 {
		// Too narrow to sit side by side: stack instead.
		return badge + "\n" + mutedStyle.Width(max(width, 16)).Render(g.Detail)
	}
	detail := mutedStyle.Width(detailW).Render(g.Detail)
	return lipgloss.JoinHorizontal(lipgloss.Top, badge, "  ", detail)
}

func renderGateBadge(g score.Gate) string {
	glyph, c := severityGlyph(g.Severity)
	text := glyph + " " + g.Title
	if g.CapTo != nil {
		text += fmt.Sprintf(" · caps %.0f", *g.CapTo)
	}
	return lipgloss.NewStyle().
		Foreground(colorBadgeInk).
		Background(c).
		Bold(true).
		Padding(0, 1).
		Render(text)
}
