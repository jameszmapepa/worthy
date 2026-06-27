package score

import (
	"strings"
	"testing"
)

func TestVerdictMentionsStrengthWeaknessAndGate(t *testing.T) {
	r := Evaluate(verdictRaw())
	v := r.Verdict

	if v == "" {
		t.Fatal("Verdict must not be empty")
	}
	// Grade letter appears.
	if !strings.Contains(v, "grade "+strings.ToLower(r.Grade)) &&
		!strings.Contains(v, "Grade "+r.Grade) &&
		!strings.Contains(v, r.Grade) {
		t.Errorf("verdict should reference the grade %q: %q", r.Grade, v)
	}
	// Highest sub-score label (a strength) appears.
	high := extremeSub(r, true)
	if !strings.Contains(strings.ToLower(v), strings.ToLower(high.Label)) {
		t.Errorf("verdict should name top strength %q: %q", high.Label, v)
	}
	// Lowest sub-score label (a weakness) appears.
	low := extremeSub(r, false)
	if !strings.Contains(strings.ToLower(v), strings.ToLower(low.Label)) {
		t.Errorf("verdict should name top weakness %q: %q", low.Label, v)
	}
}

func TestVerdictNamesTriggeredCriticalGate(t *testing.T) {
	raw := verdictRaw()
	raw.Archived = true // critical stale_or_archived gate
	r := Evaluate(raw)
	if !strings.Contains(strings.ToLower(r.Verdict), "archiv") {
		t.Errorf("verdict should surface the critical archived gate: %q", r.Verdict)
	}
}

func TestVerdictIsDeterministic(t *testing.T) {
	raw := verdictRaw()
	if Evaluate(raw).Verdict != Evaluate(raw).Verdict {
		t.Error("verdict must be deterministic for identical input")
	}
}

func TestVerdictHandlesAllNeutral(t *testing.T) {
	// An empty RawMetrics still yields a non-empty, non-panicking verdict.
	r := Evaluate(RawMetrics{})
	if r.Verdict == "" {
		t.Error("verdict must be non-empty even for empty input")
	}
}

func TestBuildVerdictNoSubScores(t *testing.T) {
	// Empty categories exercise the short return path; an unknown grade uses the
	// generic opener.
	v := buildVerdict(nil, "?", nil)
	if v == "" {
		t.Fatal("verdict must be non-empty even with no sub-scores")
	}
	if !strings.Contains(v, "?") {
		t.Errorf("verdict should still mention the grade: %q", v)
	}
}

// extremeSub returns the highest (max=true) or lowest sub-score across all
// categories, mirroring the verdict's own selection so tests stay in lockstep.
func extremeSub(r Report, max bool) SubScore {
	var pick SubScore
	first := true
	for _, c := range r.Categories {
		for _, s := range c.Subs {
			if first {
				pick, first = s, false
				continue
			}
			if (max && s.Value > pick.Value) || (!max && s.Value < pick.Value) {
				pick = s
			}
		}
	}
	return pick
}

// verdictRaw is a mixed-quality repo: strong activity, weak security.
func verdictRaw() RawMetrics {
	return RawMetrics{
		CommitsLast52Weeks:            repeat(15, 52),
		DaysSinceLastPush:             2,
		RepoAgeDays:                   1200,
		RecentIssuesClosed:            95,
		RecentIssuesOpen:              5,
		RecentPRsMerged:               40,
		RecentPRsOpen:                 1,
		MergedPRs:                     40,
		ClosedUnmergedPRs:             4,
		MedianIssueFirstResponseHours: 8,
		NewcomerPRsMerged:             6,
		NewcomerPRsClosedUnmerged:     2,
		TopContributorRecentShare:     0.4,
		ContributorCount:              12,
		ReleaseCount:                  10,
		DaysSinceLastRelease:          25,
		HasReadme:                     true,
		HasContributing:               true,
		LicenseSPDX:                   "MIT",
		HasCI:                         false, // weak security
		HasSecurityPolicy:             false,
		UsesPullRequestTarget:         false,
		WorkflowsFetched:              true,
		Stars:                         900,
		Watchers:                      150,
	}
}
