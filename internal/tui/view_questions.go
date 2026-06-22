package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// questionItem is one sub-score positioned in the two-question display order:
// grouped by question, then sorted best-to-worst within the group. An item's
// position in the flattened order is the index the model's `selected` refers to
// on this view (matching indicatorCount across both groups).
type questionItem struct {
	sub score.SubScore
	cat score.CategoryScore
}

// questionGroup is one question's verdict plus its indicators in display order.
type questionGroup struct {
	verdict score.QuestionScore
	items   []questionItem
}

// buildQuestionGroups re-groups the report's sub-scores under the two contributor
// questions (via score.QuestionVerdicts) and sorts each group's indicators
// best-to-worst by value. The groups' items, concatenated in order, form the
// flattened selection order used by the drill-down.
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
			return cmp.Compare(b.sub.Value, a.sub.Value) // descending
		})
		groups = append(groups, questionGroup{verdict: v, items: items})
	}
	return groups
}

// renderQuestions renders View 2: the report's indicators grouped under the two
// questions a prospective contributor asks — "Will it last?" and "Will my PR
// land?" — as best-to-worst horizontal bars, each under a sub-verdict grade and
// followed by the gates its indicators trigger. selected is the flattened
// indicator index across both groups (or <0 for none); expanded shows the inline
// drill-down detail below the selected row.
func renderQuestions(r score.Report, width, selected int, expanded bool) string {
	groups := buildQuestionGroups(r)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Two questions"))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  overall %s · %.1f / 100", r.Grade, r.AdjustedComposite)))
	b.WriteString("\n\n")

	total := 0
	for _, g := range groups {
		total += len(g.items)
	}
	if total == 0 {
		b.WriteString(mutedStyle.Render("(no indicators)"))
		return b.String()
	}

	barWidth := clampWidth(width-scorecardLabelWidth-44, 10, 28)
	base := 0
	for gi, g := range groups {
		b.WriteString(renderQuestionGroup(g, barWidth, width, base, selected, expanded, r.Gates))
		base += len(g.items)
		if gi < len(groups)-1 {
			b.WriteString("\n")
		}
	}

	if leftover := renderLeftoverGates(r, groups); leftover != "" {
		b.WriteString("\n")
		b.WriteString(leftover)
	}
	return b.String()
}

// renderQuestionGroup renders one question's panel: a colored sub-verdict header,
// its indicators as best-to-worst bars (with inline drill-down on the selected
// row), and the gate badges any of its indicators trigger. base is the flattened
// index of this group's first item.
func renderQuestionGroup(g questionGroup, barWidth, width, base, selected int, expanded bool, gates []score.Gate) string {
	boxW := clampWidth(width-2, 30, 200)
	textW := boxW - 2
	// Match the scorecard's per-row budget so the bar + value + raw never wrap.
	rawBudget := textW - (scorecardLabelWidth + 1 + barWidth + 1 + 5 + 2) - 1
	if rawBudget < 6 {
		rawBudget = 6
	}

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
			b.WriteString(renderDetail(it.sub, it.cat, textW))
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

// groupGates returns the report gates referenced by any indicator in the group
// (via SubScore.Gates), preserving the report's gate order.
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

// renderLeftoverGates renders gates that no displayed indicator references (e.g.
// bus_factor, vanity_stars, which key off raw metrics), so repo-level signals
// are not silently dropped.
func renderLeftoverGates(r score.Report, groups []questionGroup) string {
	referenced := map[string]bool{}
	for _, g := range groups {
		for k := range referencedGateKeys(g.items) {
			referenced[k] = true
		}
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

// referencedGateKeys is the set of gate keys referenced by the given items'
// sub-scores.
func referencedGateKeys(items []questionItem) map[string]bool {
	keys := map[string]bool{}
	for _, it := range items {
		for _, gk := range it.sub.Gates {
			keys[gk] = true
		}
	}
	return keys
}
