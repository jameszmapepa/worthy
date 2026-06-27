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
	// B3 restructured Community weights: 7 sub-scores, sum = 1.0.
	want := map[string]float64{
		"newcomer_merge_rate":  0.25, // leading direct-contribution signal
		"issue_responsiveness": 0.20, // second-strongest contribution signal
		"pr_acceptance":        0.15, // positive acceptance signal, down-weighted vs old 0.20
		"governance_docs":      0.15, // presence boolean, acts as floor not driver
		"license":              0.10, // presence boolean, acts as floor
		"pr_responsiveness":    0.10, // open-PR ghosting signal added in B3
		"newcomer_signals":     0.05, // curated-entry-point bonus added in B3
	}
	for _, s := range com.Subs {
		approx(t, s.Weight, want[s.Key], "community sub "+s.Key+" weight")
	}
}

// TestBusFactorGateCapsMaintenedQuestion reflects the B5 design: bus_factor is
// no longer a category sub-score. The risk is expressed solely through the gate,
// which caps Report.Maintained.Value ≤ 70 when share>0.80 AND count<=4
// (busFactorGateThreshold), and does NOT fire when the pool exceeds the threshold.
func TestBusFactorGateCapsMaintenedQuestion(t *testing.T) {
	// Arrange: share>0.80 AND count<=4 — gate must fire and cap Maintained.
	raw := healthyRaw()
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 3 // within busFactorGateThreshold (4)

	// Act
	r := Evaluate(raw)

	// Assert: gate is present in the report's gate list.
	if _, ok := gateByKey(r, "bus_factor"); !ok {
		t.Fatal("bus_factor gate should fire at share 0.90 / count 3 (<=threshold 4)")
	}
	// Assert: Maintained question score is capped at capBusFactor (70.0); the gate
	// must depress the question score, not silently no-op against an already-low value.
	if r.Maintained.Value > capBusFactor+eps {
		t.Errorf("Maintained.Value = %.2f, want ≤ %.0f (bus_factor gate cap)", r.Maintained.Value, capBusFactor)
	}

	// Non-trigger: count>4 — gate must NOT fire even with the same high share.
	// busFactorGateThreshold=4 means count==5 clears the gate.
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
	// A stale repo caps at 60; even an otherwise-A repo must not grade above the cap.
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400 // triggers stale (not archived) -> CapTo 60
	raw.RepoAgeDays = 100       // disable phase-downgrade (needs age>365)
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

// TestB2GateRouting_ClosedToStrangersCapsContributableOnly verifies the B2
// gate-routing rule: closed_to_strangers must cap Contributable (not Maintained)
// and leave Maintained's value equal to its RawValue.
func TestB2GateRouting_ClosedToStrangersCapsContributableOnly(t *testing.T) {
	// Arrange: trigger closed_to_strangers (pr_acceptance >= 70, newcomer_merge_rate <= 15, sample > 0).
	raw := healthyRaw()
	raw.MergedPRs = 90
	raw.ClosedUnmergedPRs = 10 // pr_acceptance = 90 >= 70
	raw.NewcomerPRsMerged = 1
	raw.NewcomerPRsClosedUnmerged = 19 // newcomer_merge_rate = 5 <= 15, sample = 20

	// Act
	r := Evaluate(raw)

	// Precondition: gate must have fired.
	if _, ok := gateByKey(r, "closed_to_strangers"); !ok {
		t.Fatal("test setup: closed_to_strangers gate did not fire")
	}

	// Assert: Contributable is capped at capStrangers (75).
	if r.Contributable.Value > capStrangers+eps {
		t.Errorf("Contributable.Value = %.2f; want ≤ %.0f (closed_to_strangers cap)",
			r.Contributable.Value, capStrangers)
	}

	// Assert: the cap actually bit — Value is lower than RawValue.
	if r.Contributable.Value >= r.Contributable.RawValue-eps && r.Contributable.RawValue > capStrangers {
		t.Errorf("cap did not reduce Contributable: RawValue=%.2f Value=%.2f",
			r.Contributable.RawValue, r.Contributable.Value)
	}

	// Assert (B2 both-directions): Maintained must NOT be capped by closed_to_strangers.
	// A healthy Activity section (healthyRaw) drives Maintained.RawValue > 75;
	// if closed_to_strangers were mis-routed onto Maintained, Value would be ≤ 75.
	if r.Maintained.Value != r.Maintained.RawValue {
		t.Errorf("Maintained.Value = %.2f, RawValue = %.2f; closed_to_strangers must not cap Maintained",
			r.Maintained.Value, r.Maintained.RawValue)
	}
}

// TestB2GateRouting_StaleGateCapsMaintenedOnly verifies the complementary B2
// direction: stale_or_archived caps Maintained (not Contributable).
func TestB2GateRouting_StaleGateCapsMaintenedOnly(t *testing.T) {
	// Arrange: stale repo (DaysSinceLastPush > 365, young enough to skip phase-downgrade).
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400
	raw.RepoAgeDays = 100 // block mature/stable phase-downgrade (needs age > 365)

	// Act
	r := Evaluate(raw)

	// Precondition: gate must have fired as warn (not info).
	g, ok := gateByKey(r, "stale_or_archived")
	if !ok {
		t.Fatal("test setup: stale_or_archived gate did not fire")
	}
	if g.Severity != SeverityWarn {
		t.Fatalf("test setup: gate severity = %q, want warn", g.Severity)
	}

	// Assert: Maintained is capped at capStale (60).
	if r.Maintained.Value > capStale+eps {
		t.Errorf("Maintained.Value = %.2f; want ≤ %.0f (stale cap)", r.Maintained.Value, capStale)
	}

	// Assert: cap actually reduced Maintained below its raw value.
	if r.Maintained.Value >= r.Maintained.RawValue-eps {
		t.Errorf("stale cap did not reduce Maintained: RawValue=%.2f Value=%.2f",
			r.Maintained.RawValue, r.Maintained.Value)
	}

	// Assert (B2 both-directions): Contributable must NOT be capped by stale_or_archived.
	if r.Contributable.Value != r.Contributable.RawValue {
		t.Errorf("Contributable.Value = %.2f, RawValue = %.2f; stale gate must not cap Contributable",
			r.Contributable.Value, r.Contributable.RawValue)
	}
}
