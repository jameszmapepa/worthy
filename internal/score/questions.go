package score

import "math"

// QuestionScore is a derived verdict answering one of the two questions the
// tool exists to answer for a prospective contributor. It is computed from the
// sub-scores that feed the question and then capped by the gates that govern
// that question, so a "Closed to newcomers" repo shows a depressed
// Contributable score regardless of its raw community numbers, and a stale
// repo shows a depressed Maintained score regardless of its historical
// activity sub-scores.
type QuestionScore struct {
	Key          string   // "maintained" | "newcomer"
	Label        string   // "Will it last?" | "Will my PR land?"
	RawValue     float64  // weighted aggregate before gate caps, 0..100, one decimal
	Value        float64  // gate-adjusted: min(RawValue, question's gate caps), 0..100
	Grade        string   // letter grade on Value (gate-adjusted)
	CategoryKeys []string // categories that feed this question, in display order
}

// questionDefs maps each contributor-facing question to the categories that
// answer it. "Will it last?" (active maintenance) is carried by Activity only;
// "Will my PR land?" (newcomer experience) by Community only. Security is not
// mapped here — it is displayed as a separate "Supply-chain integrity" section
// because integrity signals measure provenance, not project liveness.
var questionDefs = []struct {
	key, label string
	cats       []string
}{
	{"maintained", "Will it last?", []string{CategoryActivity}},
	{"newcomer", "Will my PR land?", []string{CategoryCommunity}},
}

// maintainedGateCap returns the minimum cap imposed by gates that govern the
// "Will it last?" (maintained) question. Only stale_or_archived and
// bus_factor cap this question; integrity_risk stays on the composite (it is
// a "safe to depend on" axis, not a liveness axis); vanity_stars is
// info-only. Returns 100.0 when no maintained-question gate is active.
func maintainedGateCap(gates []Gate) float64 {
	cap := 100.0
	for _, g := range gates {
		if g.CapTo == nil {
			continue
		}
		if g.Key == "stale_or_archived" || g.Key == "bus_factor" {
			if *g.CapTo < cap {
				cap = *g.CapTo
			}
		}
	}
	return cap
}

// contributableGateCap returns the minimum cap imposed by gates that govern
// the "Will my PR land?" (contributable) question. Only closed_to_strangers
// caps this question. Returns 100.0 when no contributable-question gate is
// active.
func contributableGateCap(gates []Gate) float64 {
	cap := 100.0
	for _, g := range gates {
		if g.CapTo == nil {
			continue
		}
		if g.Key == "closed_to_strangers" {
			if *g.CapTo < cap {
				cap = *g.CapTo
			}
		}
	}
	return cap
}

// computeQuestionScores aggregates the per-category values under each
// contributor question (normalized to that question's own weights), then
// applies each question's governing gate caps. The result is two first-class
// QuestionScores stored on the Report so the TUI can read them directly
// rather than re-deriving them from the category list.
//
// Gate→question routing:
//   - stale_or_archived, bus_factor  → cap Maintained
//   - closed_to_strangers            → cap Contributable
//   - integrity_risk                 → composite only (supply-chain axis)
//   - vanity_stars                   → info-only, no cap
func computeQuestionScores(cats []CategoryScore, gates []Gate) (maintained, contributable QuestionScore) {
	byKey := make(map[string]CategoryScore, len(cats))
	for _, c := range cats {
		byKey[c.Key] = c
	}

	capFns := map[string]func([]Gate) float64{
		"maintained": maintainedGateCap,
		"newcomer":   contributableGateCap,
	}

	for _, def := range questionDefs {
		var weighted, totalWeight float64
		keys := make([]string, 0, len(def.cats))
		for _, ck := range def.cats {
			c, ok := byKey[ck]
			if !ok {
				continue
			}
			keys = append(keys, ck)
			weighted += c.Weight * c.Value
			totalWeight += c.Weight
		}
		rawValue := 0.0
		if totalWeight > 0 {
			rawValue = round1(weighted / totalWeight)
		}

		capFn := capFns[def.key]
		if capFn == nil {
			capFn = func(_ []Gate) float64 { return 100.0 }
		}
		value := round1(math.Min(rawValue, capFn(gates)))

		qs := QuestionScore{
			Key:          def.key,
			Label:        def.label,
			RawValue:     rawValue,
			Value:        value,
			Grade:        LetterGrade(value),
			CategoryKeys: keys,
		}
		switch def.key {
		case "maintained":
			maintained = qs
		case "newcomer":
			contributable = qs
		}
	}
	return maintained, contributable
}

// QuestionVerdicts returns the two per-question verdicts derived from the
// report's categories and its triggered gates, in display order
// (maintained first, contributable second). It is pure and derives entirely
// from r.Categories and r.Gates so it works correctly on any Report value,
// whether produced by Evaluate or constructed by hand (e.g. in tests or TUI
// fixtures). When r has been produced by Evaluate, the returned slice is
// identical to []QuestionScore{r.Maintained, r.Contributable}.
func QuestionVerdicts(r Report) []QuestionScore {
	maintained, contributable := computeQuestionScores(r.Categories, r.Gates)
	return []QuestionScore{maintained, contributable}
}
