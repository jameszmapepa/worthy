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

	if !strings.Contains(v, "grade "+strings.ToLower(r.Grade)) &&
		!strings.Contains(v, "Grade "+r.Grade) &&
		!strings.Contains(v, r.Grade) {
		t.Errorf("verdict should reference the grade %q: %q", r.Grade, v)
	}

	high := extremeSub(r, true)
	if !strings.Contains(strings.ToLower(v), strings.ToLower(high.Label)) {
		t.Errorf("verdict should name top strength %q: %q", high.Label, v)
	}

	low := extremeSub(r, false)
	if !strings.Contains(strings.ToLower(v), strings.ToLower(low.Label)) {
		t.Errorf("verdict should name top weakness %q: %q", low.Label, v)
	}
}

func TestVerdictNamesTriggeredCriticalGate(t *testing.T) {
	raw := verdictRaw()
	raw.Archived = true
	r := Evaluate(raw)
	if !strings.Contains(strings.ToLower(r.Verdict), "archiv") {
		t.Errorf("verdict should surface the critical archived gate: %q", r.Verdict)
	}
}

func TestVerdictIsDeterministic(t *testing.T) {
	raw := verdictRaw()
	first := Evaluate(raw).Verdict
	second := Evaluate(raw).Verdict
	if first != second {
		t.Error("verdict must be deterministic for identical input")
	}
}

func TestVerdictHandlesAllNeutral(t *testing.T) {
	r := Evaluate(RawMetrics{})
	if r.Verdict == "" {
		t.Error("verdict must be non-empty even for empty input")
	}
}

func TestBuildVerdictNoSubScores(t *testing.T) {
	v := buildVerdict(nil, "?", nil)
	if v == "" {
		t.Fatal("verdict must be non-empty even with no sub-scores")
	}
	if !strings.Contains(v, "?") {
		t.Errorf("verdict should still mention the grade: %q", v)
	}
}

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
		HasCI:                         false,
		HasSecurityPolicy:             false,
		UsesPullRequestTarget:         false,
		WorkflowsFetched:              true,
		Stars:                         900,
		Watchers:                      150,
	}
}
