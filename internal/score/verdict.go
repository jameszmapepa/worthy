package score

import (
	"fmt"
	"strings"
)

var gradePhrase = map[string]string{
	"A": "Healthy and welcoming",
	"B": "In good shape",
	"C": "Mixed signals",
	"D": "Some concerns",
	"F": "Serious concerns",
}

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

func worstGate(gates []Gate) (Gate, bool) {
	var pick Gate
	best := 0
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
