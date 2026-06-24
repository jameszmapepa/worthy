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
	weightActivity  = 0.45
	weightCommunity = 0.45
	weightSecurity  = 0.10
)

// SubScore is a single scored health indicator in the range 0..100.
type SubScore struct {
	Key     string   // stable identifier, e.g. "commit_frequency"
	Label   string   // human-readable name
	Value   float64  // score in 0..100
	Raw     string   // human-readable underlying metric (e.g. "12.0 commits/wk")
	Formula string   // human-readable scoring formula (see docs/SPEC.md)
	Weight  float64  // weight of this sub-score within its category
	Gates   []string // keys of gates whose trigger condition references this sub-score
}

// subScoreGateLinks maps a sub-score key to the keys of the gates whose trigger
// condition references that sub-score (derived from the predicates in gates.go).
// It is declarative so the scorecard drill-down and Explain view can show which
// gates an indicator feeds without re-deriving the linkage. Sub-scores absent
// here carry no links. The bus_factor and vanity_stars gate predicates read raw
// metrics directly, not a sub-score value, so no sub-score links to them — the
// bus_factor sub-score grades the same signal independently of its gate.
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
	Verdict           string          // one-sentence plain-language summary
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
		busFactor(raw),
	})
	// Community is weighted (not equal): the most direct contribution signals lead
	// and the presence-boolean docs/license indicators are down-weighted so they
	// act as a floor rather than dominating the newcomer verdict.
	community := makeWeightedCategory(CategoryCommunity, "Community", weightCommunity, []SubScore{
		withWeight(issueResponsiveness(raw), 0.25),
		withWeight(prAcceptance(raw), 0.20),
		withWeight(newcomerMergeRate(raw), 0.30),
		withWeight(governanceDocs(raw), 0.15),
		withWeight(licenseScore(raw), 0.10),
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
	grade := letterGrade(adjusted)
	cats := []CategoryScore{activity, community, security}

	return Report{
		Categories:        cats,
		Composite:         composite,
		AdjustedComposite: adjusted,
		Grade:             grade,
		Gates:             gates,
		Verdict:           buildVerdict(cats, grade, gates),
	}
}

// makeCategory builds a CategoryScore as the equal-weighted average of its
// sub-scores, assigning each an equal within-category weight, then delegating to
// makeWeightedCategory so the weighted-average computation lives in one place.
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

// makeWeightedCategory builds a CategoryScore from sub-scores that already carry
// their within-category weights (expected to sum to 1.0). The category value is
// the weight-normalized average of the sub-score values. It also attaches each
// sub-score's declarative gate links.
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

// withWeight returns a copy of s with its within-category Weight set.
func withWeight(s SubScore, w float64) SubScore {
	s.Weight = w
	return s
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
