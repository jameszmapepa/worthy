package score

import "math"

// Category keys.
const (
	CategoryActivity  = "activity"
	CategoryCommunity = "community"
	CategorySecurity  = "security"
)

// Category weights in the composite. They sum to 1.0.
const (
	weightActivity  = 0.40
	weightCommunity = 0.30
	weightSecurity  = 0.30
)

// SubScore is a single scored health indicator in the range 0..100.
type SubScore struct {
	Key    string  // stable identifier, e.g. "commit_frequency"
	Label  string  // human-readable name
	Value  float64 // score in 0..100
	Raw    string  // human-readable underlying metric (e.g. "12.0 commits/wk")
	Weight float64 // weight of this sub-score within its category
}

// CategoryScore is the weighted aggregate of a category's sub-scores.
type CategoryScore struct {
	Key    string     // "activity" | "community" | "security"
	Label  string     // human-readable category name
	Value  float64    // weighted average of Subs, 0..100
	Weight float64    // category weight within the composite
	Subs   []SubScore // the indicators that make up this category
}

// Report is the complete scored result for one repository.
type Report struct {
	Categories        []CategoryScore // activity, community, security (in that order)
	Composite         float64         // raw weighted composite, rounded to one decimal
	AdjustedComposite float64         // composite after applying gate caps, one decimal
	Grade             string          // letter grade on AdjustedComposite: A/B/C/D/F
	Gates             []Gate          // conditional annotations, some of which cap
}

// Evaluate scores a RawMetrics snapshot into a Report. It is pure: the input is
// never mutated and the output depends only on the input.
func Evaluate(raw RawMetrics) Report {
	activity := makeCategory(CategoryActivity, "Activity", weightActivity, []SubScore{
		commitFrequency(raw),
		commitRecency(raw),
		releaseCadence(raw),
		issueCloseRatio(raw),
		prBacklog(raw),
	})
	community := makeCategory(CategoryCommunity, "Community", weightCommunity, []SubScore{
		issueResponsiveness(raw),
		prAcceptance(raw),
		newcomerMergeRate(raw),
		governanceDocs(raw),
		licenseScore(raw),
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

	return Report{
		Categories:        []CategoryScore{activity, community, security},
		Composite:         composite,
		AdjustedComposite: adjusted,
		Grade:             letterGrade(adjusted),
		Gates:             gates,
	}
}

// makeCategory builds a CategoryScore as the equal-weighted average of its
// sub-scores. Each sub-score carries an equal within-category weight.
func makeCategory(key, label string, weight float64, subs []SubScore) CategoryScore {
	n := len(subs)
	var sum float64
	w := 0.0
	if n > 0 {
		w = 1.0 / float64(n)
	}
	weighted := make([]SubScore, n)
	for i, s := range subs {
		s.Weight = w
		weighted[i] = s
		sum += s.Value
	}
	value := 0.0
	if n > 0 {
		value = sum / float64(n)
	}
	return CategoryScore{
		Key:    key,
		Label:  label,
		Value:  value,
		Weight: weight,
		Subs:   weighted,
	}
}

// letterGrade maps a 0..100 score to a letter grade on the spec thresholds.
func letterGrade(score float64) string {
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

// round1 rounds to one decimal place.
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// clamp constrains v to the inclusive range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
