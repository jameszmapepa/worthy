package score

import "testing"

// healthyRaw is a high-scoring repo used as a baseline that triggers no gates.
func healthyRaw() RawMetrics {
	return RawMetrics{
		CommitsLast52Weeks:            repeat(15, 52),
		DaysSinceLastPush:             1,
		RepoAgeDays:                   1000,
		RecentIssuesClosed:            90,
		RecentIssuesOpen:              10,
		RecentPRsMerged:               50,
		RecentPRsOpen:                 2,
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

	// Activity 0.45, Community 0.45, Security 0.10.
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
	approx(t, act.Weight, 0.45, "activity weight")
	approx(t, com.Weight, 0.45, "community weight")
	approx(t, sec.Weight, 0.10, "security weight")

	want := 0.45*act.Value + 0.45*com.Value + 0.10*sec.Value
	// Composite is rounded to one decimal.
	approx(t, r.Composite, round1(want), "raw composite")
}

func TestCompositeRoundedOneDecimal(t *testing.T) {
	r := Evaluate(healthyRaw())
	if r.Composite != round1(r.Composite) {
		t.Errorf("composite %v is not rounded to one decimal", r.Composite)
	}
}

func TestCategoryIsWeightedAverageOfSubs(t *testing.T) {
	r := Evaluate(healthyRaw())
	for _, c := range r.Categories {
		var sum, wsum float64
		for _, s := range c.Subs {
			sum += s.Value * s.Weight
			wsum += s.Weight
		}
		approx(t, wsum, 1.0, "category "+c.Key+" sub-weights sum to 1")
		approx(t, c.Value, sum/wsum, "category "+c.Key+" weighted average")
	}
}

// TestActivityAndSecurityAreEqualWeighted asserts the two categories that keep
// equal within-category weights still do; Community is intentionally weighted.
func TestActivityAndSecurityAreEqualWeighted(t *testing.T) {
	r := Evaluate(healthyRaw())
	for _, c := range r.Categories {
		if c.Key == CategoryCommunity {
			continue
		}
		want := 1.0 / float64(len(c.Subs))
		for _, s := range c.Subs {
			approx(t, s.Weight, want, "category "+c.Key+" sub "+s.Key+" weight")
		}
	}
}

// TestCommunityWeights pins the per-sub Community weights: the most direct
// contribution signals (newcomer_merge_rate, issue_responsiveness) lead, and the
// presence-boolean docs/license indicators are down-weighted (finding #4).
func TestCommunityWeights(t *testing.T) {
	r := Evaluate(healthyRaw())
	var com CategoryScore
	for _, c := range r.Categories {
		if c.Key == CategoryCommunity {
			com = c
		}
	}
	want := map[string]float64{
		"newcomer_merge_rate":  0.30,
		"issue_responsiveness": 0.25,
		"pr_acceptance":        0.20,
		"governance_docs":      0.15,
		"license":              0.10,
	}
	for _, s := range com.Subs {
		approx(t, s.Weight, want[s.Key], "community sub "+s.Key+" weight")
	}
}

// TestBusFactorSubScoreFillsGateBlindSpot proves finding #3's rationale: a
// fragile 0.75-share / 3-contributor repo does NOT trip the bus_factor gate
// (which needs share>0.80 AND count<=2), yet its bus_factor sub-score is
// depressed to 45 — so the risk the gate misses is no longer invisible.
func TestBusFactorSubScoreFillsGateBlindSpot(t *testing.T) {
	raw := healthyRaw()
	raw.TopContributorRecentShare = 0.75
	raw.ContributorCount = 3
	r := Evaluate(raw)

	for _, g := range r.Gates {
		if g.Key == "bus_factor" {
			t.Fatalf("bus_factor gate should not fire at share 0.75 / count 3")
		}
	}
	approx(t, findSub(t, r, "bus_factor").Value, 45, "bus_factor sub-score in gate blind spot")
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
