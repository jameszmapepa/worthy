package score

import "math"

// QuestionScore is a derived verdict answering one of the two questions the tool exists to answer for a prospective contributor.
type QuestionScore struct {
	Key          string
	Label        string
	RawValue     float64
	Value        float64
	Grade        string
	Message      string
	CategoryKeys []string
}

var maintainedMessages = map[string]string{
	"A": "Actively maintained",
	"B": "Maintained",
	"C": "Steady but slowing",
	"D": "Limited recent activity",
	"F": "Largely inactive",
}

var contributableMessages = map[string]string{
	"A": "Welcoming to newcomers",
	"B": "Open to contributions",
	"C": "Mixed for newcomers",
	"D": "Selective on outside PRs",
	"F": "Rarely merges outside PRs",
}

func questionMessage(key, grade string) string {
	switch key {
	case "maintained":
		return maintainedMessages[grade]
	case "newcomer":
		return contributableMessages[grade]
	}
	return ""
}

var questionDefs = []struct {
	key, label string
	cats       []string
}{
	{"maintained", "Will it last?", []string{CategoryActivity}},
	{"newcomer", "Will my PR land?", []string{CategoryCommunity}},
}

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

		grade := LetterGrade(value)
		qs := QuestionScore{
			Key:          def.key,
			Label:        def.label,
			RawValue:     rawValue,
			Value:        value,
			Grade:        grade,
			Message:      questionMessage(def.key, grade),
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

// QuestionVerdicts returns the two per-question verdicts in display order.
func QuestionVerdicts(r Report) []QuestionScore {
	maintained, contributable := computeQuestionScores(r.Categories, r.Gates)
	return []QuestionScore{maintained, contributable}
}
