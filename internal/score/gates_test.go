package score

import "testing"

// gateByKey returns the gate with the given key, or false if absent.
func gateByKey(r Report, key string) (Gate, bool) {
	for _, g := range r.Gates {
		if g.Key == key {
			return g, true
		}
	}
	return Gate{}, false
}

func TestBusFactorGate(t *testing.T) {
	// Trigger: top share > 0.80 AND contributors <= 2.
	raw := healthyRaw()
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 2
	r := Evaluate(raw)
	g, ok := gateByKey(r, "bus_factor")
	if !ok {
		t.Fatal("bus_factor gate should trigger")
	}
	if g.Severity != SeverityWarn {
		t.Errorf("bus_factor severity = %q, want warn", g.Severity)
	}
	if g.CapTo == nil || *g.CapTo != 70 {
		t.Errorf("bus_factor CapTo should be 70")
	}

	// Non-trigger: many contributors.
	raw2 := healthyRaw()
	raw2.TopContributorRecentShare = 0.9
	raw2.ContributorCount = 10
	if _, ok := gateByKey(Evaluate(raw2), "bus_factor"); ok {
		t.Error("bus_factor should not trigger with 10 contributors")
	}
}

func TestClosedToStrangersGate(t *testing.T) {
	// Trigger: pr_acceptance >= 70, newcomer_merge_rate <= 15, sample > 0.
	raw := healthyRaw()
	raw.MergedPRs = 90
	raw.ClosedUnmergedPRs = 10 // pr_acceptance = 90
	raw.NewcomerPRsMerged = 1
	raw.NewcomerPRsClosedUnmerged = 19 // rate = 5, sample = 20
	r := Evaluate(raw)
	g, ok := gateByKey(r, "closed_to_strangers")
	if !ok {
		t.Fatal("closed_to_strangers should trigger")
	}
	if g.CapTo == nil || *g.CapTo != 75 {
		t.Error("closed_to_strangers CapTo should be 75")
	}

	// Non-trigger: zero newcomer sample (unknown, not closed).
	raw2 := healthyRaw()
	raw2.MergedPRs = 90
	raw2.ClosedUnmergedPRs = 10
	raw2.NewcomerPRsMerged = 0
	raw2.NewcomerPRsClosedUnmerged = 0
	if _, ok := gateByKey(Evaluate(raw2), "closed_to_strangers"); ok {
		t.Error("closed_to_strangers must not trigger with zero newcomer sample")
	}
}

func TestStaleGate(t *testing.T) {
	// Stale (not archived): DaysSinceLastPush > 365 -> warn, CapTo 60.
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400
	raw.RepoAgeDays = 100 // block phase-downgrade
	g, ok := gateByKey(Evaluate(raw), "stale_or_archived")
	if !ok {
		t.Fatal("stale gate should trigger")
	}
	if g.Severity != SeverityWarn {
		t.Errorf("stale severity = %q, want warn", g.Severity)
	}
	if g.CapTo == nil || *g.CapTo != 60 {
		t.Error("stale CapTo should be 60")
	}
}

func TestArchivedGate(t *testing.T) {
	// Archived -> critical, CapTo 40.
	raw := healthyRaw()
	raw.Archived = true
	g, ok := gateByKey(Evaluate(raw), "stale_or_archived")
	if !ok {
		t.Fatal("archived gate should trigger")
	}
	if g.Severity != SeverityCritical {
		t.Errorf("archived severity = %q, want critical", g.Severity)
	}
	if g.CapTo == nil || *g.CapTo != 40 {
		t.Error("archived CapTo should be 40")
	}
}

func TestDisabledGate(t *testing.T) {
	raw := healthyRaw()
	raw.Disabled = true
	g, ok := gateByKey(Evaluate(raw), "stale_or_archived")
	if !ok {
		t.Fatal("disabled gate should trigger")
	}
	if g.Severity != SeverityCritical || g.CapTo == nil || *g.CapTo != 40 {
		t.Error("disabled -> critical, CapTo 40")
	}
}

func TestStaleNonTrigger(t *testing.T) {
	raw := healthyRaw() // pushed 1 day ago, not archived/disabled
	if _, ok := gateByKey(Evaluate(raw), "stale_or_archived"); ok {
		t.Error("stale gate must not trigger on a fresh repo")
	}
}

func TestStalePhaseDowngrade(t *testing.T) {
	// RepoAgeDays>365 AND issue_close_ratio>=70 AND ReleaseCount>0 AND not archived
	// AND stale -> downgrade to info, no cap.
	// The gate now uses the 90-day cohort close ratio from RecentIssuesClosed/Open.
	raw := healthyRaw()
	raw.DaysSinceLastPush = 400 // would be stale
	raw.RepoAgeDays = 2000
	raw.RecentIssuesClosed = 90 // cohort close ratio 90/(90+10)=90 >= 70
	raw.RecentIssuesOpen = 10
	raw.ReleaseCount = 5
	raw.Archived = false

	g, ok := gateByKey(Evaluate(raw), "stale_or_archived")
	if !ok {
		t.Fatal("phase-downgraded gate should still be present as info")
	}
	if g.Severity != SeverityInfo {
		t.Errorf("phase downgrade severity = %q, want info", g.Severity)
	}
	if g.CapTo != nil {
		t.Error("phase-downgraded gate must not cap")
	}
}

func TestPhaseDowngradeDoesNotApplyToArchived(t *testing.T) {
	// Archived repos never get the mature/stable downgrade.
	raw := healthyRaw()
	raw.Archived = true
	raw.RepoAgeDays = 2000
	raw.RecentIssuesClosed = 90
	raw.RecentIssuesOpen = 10
	raw.ReleaseCount = 5
	g, _ := gateByKey(Evaluate(raw), "stale_or_archived")
	if g.Severity != SeverityCritical {
		t.Error("archived must stay critical even when mature/stable conditions hold")
	}
}

func TestIntegrityRiskGate(t *testing.T) {
	// UsesPullRequestTarget && !HasSignedReleaseAssets && rawComposite > 70.
	raw := healthyRaw()
	raw.UsesPullRequestTarget = true
	raw.WorkflowsFetched = true
	raw.HasSignedReleaseAssets = false
	r := Evaluate(raw)
	if r.Composite <= 70 {
		t.Fatalf("test setup: expected rawComposite > 70, got %v", r.Composite)
	}
	g, ok := gateByKey(r, "integrity_risk")
	if !ok {
		t.Fatal("integrity_risk should trigger")
	}
	if g.CapTo == nil || *g.CapTo != 80 {
		t.Error("integrity_risk CapTo should be 80")
	}

	// Non-trigger: signed assets present.
	raw2 := healthyRaw()
	raw2.UsesPullRequestTarget = true
	raw2.HasSignedReleaseAssets = true
	if _, ok := gateByKey(Evaluate(raw2), "integrity_risk"); ok {
		t.Error("integrity_risk must not trigger when releases are signed")
	}
}

func TestVanityStarsGate(t *testing.T) {
	// Stars>5000 && Watchers*200 < Stars -> info, no cap.
	raw := healthyRaw()
	raw.Stars = 50000
	raw.Watchers = 100 // 100*200 = 20000 < 50000
	g, ok := gateByKey(Evaluate(raw), "vanity_stars")
	if !ok {
		t.Fatal("vanity_stars should trigger")
	}
	if g.Severity != SeverityInfo {
		t.Errorf("vanity_stars severity = %q, want info", g.Severity)
	}
	if g.CapTo != nil {
		t.Error("vanity_stars must not cap")
	}

	// Non-trigger: healthy watcher ratio.
	raw2 := healthyRaw()
	raw2.Stars = 50000
	raw2.Watchers = 1000 // 1000*200 = 200000 > 50000
	if _, ok := gateByKey(Evaluate(raw2), "vanity_stars"); ok {
		t.Error("vanity_stars must not trigger with healthy watcher ratio")
	}
}

func TestAdjustedCompositeTakesMinCap(t *testing.T) {
	// Two capping gates active -> adjusted = min(rawComposite, min cap).
	raw := healthyRaw()
	raw.Archived = true // CapTo 40
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 1 // bus_factor CapTo 70
	r := Evaluate(raw)
	if r.AdjustedComposite > 40+eps {
		t.Errorf("adjusted composite %v should be capped at 40 (the lower cap)", r.AdjustedComposite)
	}
}

func TestAdjustedNeverExceedsRawComposite(t *testing.T) {
	// When a cap is higher than the raw composite, raw composite wins.
	raw := healthyRaw()
	raw.HasCI = false
	raw.HasSecurityPolicy = false
	raw.LicenseSPDX = ""
	raw.HasReadme = false
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 1 // bus_factor cap 70, but composite already low
	r := Evaluate(raw)
	if r.AdjustedComposite > r.Composite+eps {
		t.Errorf("adjusted %v must never exceed raw composite %v", r.AdjustedComposite, r.Composite)
	}
}

func TestEveryGateHasDetail(t *testing.T) {
	raw := healthyRaw()
	raw.Archived = true
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 1
	raw.Stars = 50000
	raw.Watchers = 1
	for _, g := range Evaluate(raw).Gates {
		if g.Detail == "" {
			t.Errorf("gate %q missing Detail", g.Key)
		}
		if g.Title == "" {
			t.Errorf("gate %q missing Title", g.Key)
		}
	}
}
