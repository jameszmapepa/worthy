package score

import "math"

// Category key constants identify the three scoring dimensions.
const (
	CategoryActivity  = "activity"
	CategoryCommunity = "community"
	CategorySecurity  = "security"
)

const (
	weightActivity  = 0.475
	weightCommunity = 0.45
	weightSecurity  = 0.075
)

// SubScore is a single scored health indicator in the range 0..100.
type SubScore struct {
	Key     string
	Label   string
	Value   float64
	Raw     string
	Formula string
	Weight  float64
	Gates   []string
}

var subScoreGateLinks = map[string][]string{
	"pr_acceptance":       {"closed_to_strangers"},
	"newcomer_merge_rate": {"closed_to_strangers"},
	"commit_recency":      {"stale_or_archived"},
	"issue_close_ratio":   {"stale_or_archived"},
	"release_cadence":     {"stale_or_archived"},
	"workflow_safety":     {"integrity_risk"},
	"signed_releases":     {"integrity_risk"},
}

// CategoryScore is the weighted aggregate of a category's sub-scores.
type CategoryScore struct {
	Key    string
	Label  string
	Value  float64
	Weight float64
	Subs   []SubScore
}

// Report is the complete scored result for one repository.
type Report struct {
	Categories        []CategoryScore
	Composite         float64
	AdjustedComposite float64
	Grade             string
	Gates             []Gate
	Verdict           string

	Maintained    QuestionScore
	Contributable QuestionScore

	Confidence ConfidenceLevel
}

// Evaluate scores a RawMetrics snapshot into a Report; pure, deterministic, and performs no I/O.
func Evaluate(raw RawMetrics) Report {
	activity := makeCategory(CategoryActivity, "Activity", weightActivity, []SubScore{
		commitFrequency(raw),
		commitRecency(raw),
		releaseCadence(raw),
		issueCloseRatio(raw),
		prBacklog(raw),
	})

	community := makeWeightedCategory(CategoryCommunity, "Community", weightCommunity, []SubScore{
		withWeight(newcomerMergeRate(raw), 0.25),
		withWeight(issueResponsiveness(raw), 0.20),
		withWeight(prAcceptance(raw), 0.15),
		withWeight(governanceDocs(raw), 0.15),
		withWeight(licenseScore(raw), 0.10),
		withWeight(prResponsiveness(raw), 0.10),
		withWeight(newcomerSignals(raw), 0.05),
	})

	security := makeCategory(CategorySecurity, "Security", weightSecurity, []SubScore{
		ciPresent(raw),
		signedReleases(raw),
		securityPolicy(raw),
		workflowSafety(raw),
	})

	composite := round1(
		weightActivity*activity.Value +
			weightCommunity*community.Value +
			weightSecurity*security.Value,
	)

	gates := evaluateGates(raw, composite, subLookup{
		issueCloseRatio:   issueCloseRatio(raw).Value,
		prAcceptance:      prAcceptance(raw).Value,
		newcomerMergeRate: newcomerMergeRate(raw).Value,
	})

	adjusted := round1(applyCaps(composite, gates))
	grade := LetterGrade(adjusted)
	cats := []CategoryScore{activity, community, security}

	maintained, contributable := computeQuestionScores(cats, gates)

	return Report{
		Categories:        cats,
		Composite:         composite,
		AdjustedComposite: adjusted,
		Grade:             grade,
		Gates:             gates,
		Verdict:           buildVerdict(cats, grade, gates),
		Maintained:        maintained,
		Contributable:     contributable,
		Confidence:        computeConfidence(raw),
	}
}

func makeCategory(key, label string, weight float64, subs []SubScore) CategoryScore {
	n := len(subs)
	w := 0.0
	if n > 0 {
		w = 1.0 / float64(n)
	}
	weighted := make([]SubScore, n)
	for i, s := range subs {
		weighted[i] = withWeight(s, w)
	}
	return makeWeightedCategory(key, label, weight, weighted)
}

func makeWeightedCategory(key, label string, weight float64, subs []SubScore) CategoryScore {
	var sum, wsum float64
	weighted := make([]SubScore, len(subs))
	for i, s := range subs {
		s.Gates = subScoreGateLinks[s.Key]
		weighted[i] = s
		sum += s.Value * s.Weight
		wsum += s.Weight
	}
	value := 0.0
	if wsum > 0 {
		value = sum / wsum
	}
	return CategoryScore{
		Key:    key,
		Label:  label,
		Value:  value,
		Weight: weight,
		Subs:   weighted,
	}
}

func withWeight(s SubScore, w float64) SubScore {
	s.Weight = w
	return s
}

// LetterGrade maps a 0..100 score to a letter grade; callers must use this function rather than duplicating thresholds.
func LetterGrade(score float64) string {
	switch {
	case score >= 85:
		return "A"
	case score >= 70:
		return "B"
	case score >= 55:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
