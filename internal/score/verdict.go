package score

import (
	"fmt"
	"strings"
)

// gradePhrase gives a plain-language opener for each letter grade. Phrased
// diplomatically and around the tool's two questions (is it maintained / open
// to contribution) rather than a clinical "health" label.
var gradePhrase = map[string]string{
	"A": "Healthy and welcoming",
	"B": "In good shape",
	"C": "Mixed signals",
	"D": "Some concerns",
	"F": "Serious concerns",
}

// buildVerdict composes a one-sentence, honest summary of a report from its
// grade, strongest and weakest indicators, and worst triggered gate. It is
// pure and deterministic: ties are broken by category/sub-score order.
func buildVerdict(cats []CategoryScore, grade string, gates []Gate) string {
	opener := gradePhrase[grade]
	if opener == "" {
		opener = "Scored"
	}

	high, low, ok := extremes(cats)
	if !ok {
		return fmt.Sprintf("%s (grade %s).", opener, grade)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (grade %s): strongest on %s, weakest on %s",
		opener, grade, strings.ToLower(high.Label), strings.ToLower(low.Label))

	if g, found := worstGate(gates); found {
		fmt.Fprintf(&b, "; flagged %s", strings.ToLower(g.Title))
	}
	b.WriteString(".")
	return b.String()
}

// extremes returns the highest- and lowest-valued sub-scores across all
// categories. ok is false when there are no sub-scores. Ties keep the first
// seen, so the result is deterministic.
func extremes(cats []CategoryScore) (high, low SubScore, ok bool) {
	first := true
	for _, c := range cats {
		for _, s := range c.Subs {
			if first {
				high, low, first = s, s, false
				continue
			}
			if s.Value > high.Value {
				high = s
			}
			if s.Value < low.Value {
				low = s
			}
		}
	}
	return high, low, !first
}

// worstGate returns the most severe triggered gate (critical > warn), ignoring
// info gates. found is false when no warn/critical gate is present.
func worstGate(gates []Gate) (Gate, bool) {
	var pick Gate
	best := 0 // 0 none, 1 warn, 2 critical
	for _, g := range gates {
		rank := 0
		switch g.Severity {
		case SeverityCritical:
			rank = 2
		case SeverityWarn:
			rank = 1
		}
		if rank > best {
			best, pick = rank, g
		}
	}
	return pick, best > 0
}
