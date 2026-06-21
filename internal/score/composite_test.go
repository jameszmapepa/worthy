package score

import "testing"

// healthyRaw is a high-scoring repo used as a baseline that triggers no gates.
func healthyRaw() RawMetrics {
	return RawMetrics{
		CommitsLast52Weeks:            repeat(15, 52),
		DaysSinceLastPush:             1,
		RepoAgeDays:                   1000,
		OpenIssues:                    10,
		ClosedIssues:                  90,
		OpenPRs:                       2,
		MergedPRs:                     50,
		ClosedUnmergedPRs:             5,
		MedianIssueFirstResponseHours: 6,
		NewcomerPRsMerged:             8,
		NewcomerPRsClosedUnmerged:     2,
		TopContributorRecentShare:     0.3,
		ContributorCount:              20,
		ReleaseCount:                  12,
		DaysSinceLastRelease:          20,
		HasCI:                         true,
		HasSignedReleaseAssets:        true,
		HasSecurityPolicy:             true,
		HealthPercentage:              100,
		HasReadme:                     true,
		HasContributing:               true,
		HasCodeOfConduct:              true,
		HasLicense:                    true,
		LicenseSPDX:                   "MIT",
		UsesPullRequestTarget:         false,
		WorkflowsFetched:              true,
		Stars:                         1000,
		Forks:                         100,
		Watchers:                      200,
	}
}

func TestCategoryWeightsAndComposite(t *testing.T) {
	r := Evaluate(healthyRaw())

	// Activity 0.40, Community 0.30, Security 0.30.
	var act, com, sec CategoryScore
	for _, c := range r.Categories {
		switch c.Key {
		case "activity":
			act = c
		case "community":
			com = c
		case "security":
			sec = c
		}
	}
	approx(t, act.Weight, 0.40, "activity weight")
	approx(t, com.Weight, 0.30, "community weight")
	approx(t, sec.Weight, 0.30, "security weight")

	want := 0.40*act.Value + 0.30*com.Value + 0.30*sec.Value
	// Composite is rounded to one decimal.
	approx(t, r.Composite, round1(want), "raw composite")
}

func TestCompositeRoundedOneDecimal(t *testing.T) {
	r := Evaluate(healthyRaw())
	if r.Composite != round1(r.Composite) {
		t.Errorf("composite %v is not rounded to one decimal", r.Composite)
	}
}

func TestCategoryIsEqualWeightedAverage(t *testing.T) {
	r := Evaluate(healthyRaw())
	for _, c := range r.Categories {
		var sum float64
		for _, s := range c.Subs {
			sum += s.Value
		}
		mean := sum / float64(len(c.Subs))
		approx(t, c.Value, mean, "category "+c.Key+" mean")
	}
}

func TestLetterGrade(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{100, "A"},
		{85, "A"},
		{84.9, "B"},
		{70, "B"},
		{69.9, "C"},
		{55, "C"},
		{54.9, "D"},
		{40, "D"},
		{39.9, "F"},
		{0, "F"},
	}
	for _, tc := range tests {
		if got := letterGrade(tc.score); got != tc.want {
			t.Errorf("letterGrade(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestGradeUsesAdjustedComposite(t *testing.T) {
	// A stale repo caps at 60; even an otherwise-A repo must not grade above the cap.
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400 // triggers stale (not archived) -> CapTo 60
	raw.RepoAgeDays = 100       // disable phase-downgrade (needs age>365)
	r := Evaluate(raw)

	if r.AdjustedComposite > 60+eps {
		t.Errorf("adjusted composite %v exceeds stale cap 60", r.AdjustedComposite)
	}
	if r.Grade != letterGrade(r.AdjustedComposite) {
		t.Errorf("grade %q not derived from adjusted composite %v", r.Grade, r.AdjustedComposite)
	}
}

func TestNoGatesMeansAdjustedEqualsComposite(t *testing.T) {
	r := Evaluate(healthyRaw())
	for _, g := range r.Gates {
		if g.CapTo != nil {
			t.Fatalf("healthy repo unexpectedly triggered capping gate %q", g.Key)
		}
	}
	approx(t, r.AdjustedComposite, r.Composite, "adjusted == composite")
}
