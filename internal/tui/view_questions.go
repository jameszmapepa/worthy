package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

type questionItem struct {
	sub score.SubScore
	cat score.CategoryScore
}

type questionGroup struct {
	verdict score.QuestionScore
	items   []questionItem
}

func buildQuestionGroups(r score.Report) []questionGroup {
	byKey := make(map[string]score.CategoryScore, len(r.Categories))
	for _, c := range r.Categories {
		byKey[c.Key] = c
	}
	verdicts := score.QuestionVerdicts(r)
	groups := make([]questionGroup, 0, len(verdicts))
	for _, v := range verdicts {
		items := make([]questionItem, 0)
		for _, ck := range v.CategoryKeys {
			cat := byKey[ck]
			for _, s := range cat.Subs {
				items = append(items, questionItem{sub: s, cat: cat})
			}
		}
		slices.SortStableFunc(items, func(a, b questionItem) int {
			return cmp.Compare(b.sub.Value, a.sub.Value)
		})
		groups = append(groups, questionGroup{verdict: v, items: items})
	}
	return groups
}

func integrityItems(r score.Report) []questionItem {
	for _, c := range r.Categories {
		if c.Key != score.CategorySecurity {
			continue
		}
		items := make([]questionItem, len(c.Subs))
		for i, s := range c.Subs {
			items[i] = questionItem{sub: s, cat: c}
		}
		slices.SortStableFunc(items, func(a, b questionItem) int {
			return cmp.Compare(b.sub.Value, a.sub.Value)
		})
		return items
	}
	return nil
}

func renderQuestions(r score.Report, width, selected int, expanded bool) string {
	groups := buildQuestionGroups(r)
	integrity := integrityItems(r)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Two questions"))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  overall %s · %.1f / 100", r.Grade, r.AdjustedComposite)))
	b.WriteString("\n\n")

	total := 0
	for _, g := range groups {
		total += len(g.items)
	}
	total += len(integrity)
	if total == 0 {
		b.WriteString(mutedStyle.Render("(no indicators)"))
		return b.String()
	}

	barWidth := clampWidth(width-scorecardLabelWidth-scorecardBarWidthOverhead, 10, 28)
	base := 0
	for gi, g := range groups {
		b.WriteString(renderQuestionGroup(g, barWidth, width, base, selected, expanded, r.Gates))
		base += len(g.items)
		if gi < len(groups)-1 || len(integrity) > 0 {
			b.WriteString("\n")
		}
	}

	if len(integrity) > 0 {
		b.WriteString(renderIntegritySection(r, integrity, barWidth, width, base, selected, expanded))
	}

	if leftover := renderLeftoverGates(r, groups, integrity); leftover != "" {
		b.WriteString("\n")
		b.WriteString(leftover)
	}
	return b.String()
}

func renderIntegritySection(r score.Report, items []questionItem, barWidth, width, base, selected int, expanded bool) string {
	var secCat score.CategoryScore
	for _, c := range r.Categories {
		if c.Key == score.CategorySecurity {
			secCat = c
			break
		}
	}

	boxW := clampWidth(width-2, 30, 200)
	textW := boxW - 2
	rawBudget := max(textW-(scorecardLabelWidth+1+barWidth+1+5+1+2)-1, 6)

	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(barColor(secCat.Value)).Bold(true).
		Render("Supply-chain integrity")

	b.WriteString(header)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  %s · %.0f / 100", score.LetterGrade(secCat.Value), secCat.Value)))
	b.WriteString("\n")

	for i, it := range items {
		sel := base+i == selected
		b.WriteString(renderSubLine(it.sub, barWidth, rawBudget, sel))
		b.WriteString("\n")
		if sel && expanded {
			b.WriteString(renderDetail(it.sub, it.cat))
			b.WriteString("\n")
		}
	}

	keys := referencedGateKeys(items)
	for _, gt := range r.Gates {
		if keys[gt.Key] {
			b.WriteString(renderGateBadge(gt))
			b.WriteString("  ")
			b.WriteString(mutedStyle.Render(gt.Detail))
			b.WriteString("\n")
		}
	}

	return panelStyle.Width(boxW).Render(strings.TrimRight(b.String(), "\n"))
}

func renderQuestionGroup(g questionGroup, barWidth, width, base, selected int, expanded bool, gates []score.Gate) string {
	boxW := clampWidth(width-2, 30, 200)
	textW := boxW - 2

	rawBudget := max(textW-(scorecardLabelWidth+1+barWidth+1+5+1+2)-1, 6)

	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(barColor(g.verdict.Value)).Bold(true).
		Render(g.verdict.Label)

	b.WriteString(header)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  %s · %.0f / 100", g.verdict.Grade, g.verdict.Value)))
	b.WriteString("\n")

	for i, it := range g.items {
		sel := base+i == selected
		b.WriteString(renderSubLine(it.sub, barWidth, rawBudget, sel))
		b.WriteString("\n")
		if sel && expanded {
			b.WriteString(renderDetail(it.sub, it.cat))
			b.WriteString("\n")
		}
	}

	for _, gt := range groupGates(g, gates) {
		b.WriteString(renderGateBadge(gt))
		b.WriteString("  ")
		b.WriteString(mutedStyle.Render(gt.Detail))
		b.WriteString("\n")
	}

	return panelStyle.Width(boxW).Render(strings.TrimRight(b.String(), "\n"))
}

func groupGates(g questionGroup, gates []score.Gate) []score.Gate {
	keys := referencedGateKeys(g.items)
	var out []score.Gate
	for _, gt := range gates {
		if keys[gt.Key] {
			out = append(out, gt)
		}
	}
	return out
}

func renderLeftoverGates(r score.Report, groups []questionGroup, integrityItems []questionItem) string {
	referenced := map[string]bool{}
	for _, g := range groups {
		for k := range referencedGateKeys(g.items) {
			referenced[k] = true
		}
	}
	for k := range referencedGateKeys(integrityItems) {
		referenced[k] = true
	}
	var b strings.Builder
	for _, gt := range r.Gates {
		if referenced[gt.Key] {
			continue
		}
		b.WriteString(renderGateBadge(gt))
		b.WriteString("  ")
		b.WriteString(mutedStyle.Render(gt.Detail))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func referencedGateKeys(items []questionItem) map[string]bool {
	keys := map[string]bool{}
	for _, it := range items {
		for _, gk := range it.sub.Gates {
			keys[gk] = true
		}
	}
	return keys
}
