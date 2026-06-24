package score

import "testing"

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
	approx(t, act.Weight, 0.475, "activity weight")
	approx(t, com.Weight, 0.45, "community weight")
	approx(t, sec.Weight, 0.075, "security weight")

	want := act.Weight*act.Value + com.Weight*com.Value + sec.Weight*sec.Value

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

func TestCommunityWeights(t *testing.T) {
	r := Evaluate(healthyRaw())
	var com CategoryScore
	for _, c := range r.Categories {
		if c.Key == CategoryCommunity {
			com = c
		}
	}

	want := map[string]float64{
		"newcomer_merge_rate":  0.25,
		"issue_responsiveness": 0.20,
		"pr_acceptance":        0.15,
		"governance_docs":      0.15,
		"license":              0.10,
		"pr_responsiveness":    0.10,
		"newcomer_signals":     0.05,
	}
	for _, s := range com.Subs {
		approx(t, s.Weight, want[s.Key], "community sub "+s.Key+" weight")
	}
}

func TestBusFactorGateCapsMaintenedQuestion(t *testing.T) {
	raw := healthyRaw()
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 3

	r := Evaluate(raw)

	if _, ok := gateByKey(r, "bus_factor"); !ok {
		t.Fatal("bus_factor gate should fire at share 0.90 / count 3 (<=threshold 4)")
	}

	if r.Maintained.Value > capBusFactor+eps {
		t.Errorf("Maintained.Value = %.2f, want ≤ %.0f (bus_factor gate cap)", r.Maintained.Value, capBusFactor)
	}

	raw2 := healthyRaw()
	raw2.TopContributorRecentShare = 0.9
	raw2.ContributorCount = 5
	if _, fired := gateByKey(Evaluate(raw2), "bus_factor"); fired {
		t.Error("bus_factor gate must not fire when ContributorCount > 4 (busFactorGateThreshold)")
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
		if got := LetterGrade(tc.score); got != tc.want {
			t.Errorf("LetterGrade(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestGradeUsesAdjustedComposite(t *testing.T) {
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400
	raw.RepoAgeDays = 100
	r := Evaluate(raw)

	if r.AdjustedComposite > 60+eps {
		t.Errorf("adjusted composite %v exceeds stale cap 60", r.AdjustedComposite)
	}
	if r.Grade != LetterGrade(r.AdjustedComposite) {
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

func TestB2GateRouting_ClosedToStrangersCapsContributableOnly(t *testing.T) {
	raw := healthyRaw()
	raw.MergedPRs = 90
	raw.ClosedUnmergedPRs = 10
	raw.NewcomerPRsMerged = 1
	raw.NewcomerPRsClosedUnmerged = 19

	r := Evaluate(raw)

	if _, ok := gateByKey(r, "closed_to_strangers"); !ok {
		t.Fatal("test setup: closed_to_strangers gate did not fire")
	}

	if r.Contributable.Value > capStrangers+eps {
		t.Errorf("Contributable.Value = %.2f; want ≤ %.0f (closed_to_strangers cap)",
			r.Contributable.Value, capStrangers)
	}

	if r.Contributable.Value >= r.Contributable.RawValue-eps && r.Contributable.RawValue > capStrangers {
		t.Errorf("cap did not reduce Contributable: RawValue=%.2f Value=%.2f",
			r.Contributable.RawValue, r.Contributable.Value)
	}

	if r.Maintained.Value != r.Maintained.RawValue {
		t.Errorf("Maintained.Value = %.2f, RawValue = %.2f; closed_to_strangers must not cap Maintained",
			r.Maintained.Value, r.Maintained.RawValue)
	}
}

func TestB2GateRouting_StaleGateCapsMaintenedOnly(t *testing.T) {
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400
	raw.RepoAgeDays = 100

	r := Evaluate(raw)

	g, ok := gateByKey(r, "stale_or_archived")
	if !ok {
		t.Fatal("test setup: stale_or_archived gate did not fire")
	}
	if g.Severity != SeverityWarn {
		t.Fatalf("test setup: gate severity = %q, want warn", g.Severity)
	}

	if r.Maintained.Value > capStale+eps {
		t.Errorf("Maintained.Value = %.2f; want ≤ %.0f (stale cap)", r.Maintained.Value, capStale)
	}

	if r.Maintained.Value >= r.Maintained.RawValue-eps {
		t.Errorf("stale cap did not reduce Maintained: RawValue=%.2f Value=%.2f",
			r.Maintained.RawValue, r.Maintained.Value)
	}

	if r.Contributable.Value != r.Contributable.RawValue {
		t.Errorf("Contributable.Value = %.2f, RawValue = %.2f; stale gate must not cap Contributable",
			r.Contributable.Value, r.Contributable.RawValue)
	}
}
