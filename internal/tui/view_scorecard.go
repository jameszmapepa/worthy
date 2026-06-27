package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// scorecardLabelWidth is the fixed column width for sub-score labels.
const scorecardLabelWidth = 22

// scorecardBarWidthOverhead is the total fixed column overhead in a category
// panel row, excluding the bar and raw-metric fields. It accounts for: the
// panel's border (2) + padding (2), the selection marker (2), the post-label
// space (1), the post-bar space (1), the numeric value (5), the text-tier grade
// (1, C5), the pre-raw spaces (2), and the rawBudget floor (6).
// Together with scorecardLabelWidth it determines the usable bar width for any
// terminal width: barWidth = clampWidth(width − scorecardLabelWidth −
// scorecardBarWidthOverhead, 10, 28).
const scorecardBarWidthOverhead = 44

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

// renderScorecard renders View 1. The layout, from top to bottom:
//  1. Two question-answer headline cards (C2 — the product's core value).
//  2. The composite hero block (overall score + grade + verdict).
//  3. Per-category bordered panels of indicator bars.
//  4. Gate badges.
//
// When selected >= 0 the indicator at that flattened index is highlighted;
// when expanded is also true, an inline detail panel is rendered below it.
func renderScorecard(r score.Report, width, selected int, expanded bool) string {
	var b strings.Builder

	// C2: question headline cards — rendered first so they dominate the view.
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

// renderQuestionCards renders the two core question answers ("Will it last?"
// and "Will my PR land?") as prominent bordered headline cards. On terminals
// narrower than narrowTerminalWidth the cards stack vertically; otherwise they
// sit side by side. A confidence caveat is appended when data is sparse (C2).
func renderQuestionCards(r score.Report, width int) string {
	// Allocate roughly half the available width to each card, accounting for
	// the two-space gap between them and the outer border overhead.
	halfW := (width - 6) / 2
	if halfW < 22 {
		halfW = 22
	}

	card1 := renderQuestionCard(r.Maintained, halfW)
	card2 := renderQuestionCard(r.Contributable, halfW)

	var cards string
	if width < narrowTerminalWidth {
		cards = lipgloss.JoinVertical(lipgloss.Left, card1, card2)
	} else {
		cards = lipgloss.JoinHorizontal(lipgloss.Top, card1, "  ", card2)
	}

	// Confidence caveat: only emitted for Low or Medium so High-confidence
	// reports (the common case) stay uncluttered.
	caveat := confidenceCaveat(r.Confidence)
	if caveat == "" {
		return cards
	}
	return lipgloss.JoinVertical(lipgloss.Left, cards, caveat)
}

// renderQuestionCard renders one question's headline card showing the question
// label, the gate-adjusted grade, the numeric value, and a minimal description.
func renderQuestionCard(qs score.QuestionScore, width int) string {
	// Width(w) in lipgloss includes padding but not border; the thick border
	// adds 2 cells outside. innerW is the usable text width.
	innerW := width - 6 // border(2) + padding(4)
	if innerW < 10 {
		innerW = 10
	}

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
	// Width(w) sets the box including padding; border is outside.
	return questionCardStyle.Width(width - 2).Render(body)
}

// confidenceCaveat returns a muted caveat string for Low/Medium confidence so a
// low-signal repo does not look falsely precise. Returns "" for High confidence.
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
	// C5 adds 1 char (text-tier grade letter) to the row; rawBudget is reduced
	// accordingly so the row never wraps inside its panel.
	rawBudget := textW - (scorecardLabelWidth + 1 + barWidth + 1 + 5 + 1 + 2) - 1
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
// label, colored bar, numeric value, text-tier grade letter (C5 — survives
// NO_COLOR), and the (truncated) raw metric so the row never wraps. The
// selected row is highlighted.
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
	// C5: text-tier grade letter so quality level is legible in monochrome /
	// NO_COLOR terminals where lipgloss strips color from the bar and value.
	grade := mutedStyle.Render(score.LetterGrade(s.Value))
	raw := mutedStyle.Render(truncate(s.Raw, rawBudget))
	return fmt.Sprintf("%s%s %s %s%s  %s", marker, label, bar, value, grade, raw)
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
