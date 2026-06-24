package score

// QuestionScore is a derived verdict answering one of the two questions the tool
// exists to answer for a prospective contributor, aggregated from the categories
// that feed it. It is a presentation re-grouping of the existing categories: no
// new weights or thresholds are introduced.
type QuestionScore struct {
	Key          string   // "maintained" | "newcomer"
	Label        string   // "Will it last?" | "Will my PR land?"
	Value        float64  // weighted aggregate of its categories, 0..100, one decimal
	Grade        string   // letter grade on Value
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

// QuestionVerdicts re-groups a Report's categories under the two contributor
// questions, weighting each question's categories by their existing composite
// weights (normalized within the question). It is pure and derives entirely from
// r.Categories; categories absent from the report are skipped.
func QuestionVerdicts(r Report) []QuestionScore {
	byKey := make(map[string]CategoryScore, len(r.Categories))
	for _, c := range r.Categories {
		byKey[c.Key] = c
	}

	out := make([]QuestionScore, 0, len(questionDefs))
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
		value := 0.0
		if totalWeight > 0 {
			value = round1(weighted / totalWeight)
		}
		out = append(out, QuestionScore{
			Key:          def.key,
			Label:        def.label,
			Value:        value,
			Grade:        letterGrade(value),
			CategoryKeys: keys,
		})
	}
	return out
}
