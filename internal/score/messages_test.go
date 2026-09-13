package score

import (
	"strings"
	"testing"
)

func activeRaw() RawMetrics {
	weeks := make([]int, 52)
	for i := range weeks {
		weeks[i] = 12
	}
	return RawMetrics{
		CommitsLast52Weeks: weeks, DaysSinceLastPush: 3, ReleaseCount: 10, DaysSinceLastRelease: 20,
		NewcomerPRsMerged: 9, NewcomerPRsClosedUnmerged: 1, MedianIssueFirstResponseHours: 8,
		MergedPRs: 80, ClosedUnmergedPRs: 20, RecentIssuesClosed: 40, RecentIssuesOpen: 5,
		RecentPRsMerged: 30, RecentPRsOpen: 3, OpenPRCount: 3, MedianOpenPRAgeDays: 5,
		HasReadme: true, HasContributing: true, HasCodeOfConduct: true, HasSecurityPolicy: true,
		LicenseSPDX: "MIT", HasCI: true, HasSignedReleaseAssets: true, WorkflowsFetched: true,
		NewcomerLabelsAvailable: true, NewcomerLabeledAvailable: 3, NewcomerLabeledOpen: 3,
		ContributorCount: 20, TopContributorRecentShare: 0.3, RepoAgeDays: 900, Stars: 100, Watchers: 10,
	}
}

func TestMaintainedMessagePerGrade(t *testing.T) {
	raw := activeRaw()
	cases := map[string][]string{
		"A": {"Pushed 3d ago", "12 default-branch commits a week", "last release 20d ago"},
		"B": {"Active:", "pushed 3d ago"},
		"C": {"Mixed:"},
		"D": {"Weak:"},
		"F": {"Dormant:", "Assume no one will review your PR"},
	}
	for grade, wants := range cases {
		in := raw
		if grade == "F" {
			in.DaysSinceLastPush = 400
		}
		got := maintainedMessage(grade, in)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("grade %s: %q missing %q", grade, got, w)
			}
		}
		if !strings.HasSuffix(got, ".") {
			t.Errorf("grade %s: message should end with a full stop: %q", grade, got)
		}
	}
}

func TestMaintainedMessageQuietFacts(t *testing.T) {
	raw := RawMetrics{CommitsLast52Weeks: make([]int, 52), DaysSinceLastPush: 130, ReleaseCount: 0}
	got := maintainedMessage("D", raw)
	for _, w := range []string{"pushed 4mo ago", "no default-branch commits most weeks", "no releases"} {
		if !strings.Contains(got, w) {
			t.Errorf("%q missing %q", got, w)
		}
	}
	if got := maintainedMessage("F", RawMetrics{DaysSinceLastPush: 420}); !strings.Contains(got, "pushed 14mo ago") {
		t.Errorf("unavailable commit stats should still report the push age: %q", got)
	}
	if got := maintainedMessage("F", RawMetrics{DaysSinceLastPush: 2}); strings.HasPrefix(got, "Dormant") {
		t.Errorf("a repo pushed 2 days ago is not dormant: %q", got)
	}
}

func TestContributableMessagePerGrade(t *testing.T) {
	raw := activeRaw()
	raw.StaleNewcomerOpenPRs = 12
	cases := map[string][]string{
		"A": {"Yes:", "9 of 10 newcomer PRs merged", "first reply in about 8h", "12 newcomer PRs waiting over 30d", "contributing guide present"},
		"B": {"Likely:"},
		"C": {"Unclear:", "Check the open PR queue"},
		"D": {"Doubtful:", "get a yes before coding"},
		"F": {"No: outside PRs are almost never merged", "Fork or move on"},
	}
	for grade, wants := range cases {
		got := contributableMessage(grade, raw)
		for _, w := range wants {
			if !strings.Contains(got, w) {
				t.Errorf("grade %s: %q missing %q", grade, got, w)
			}
		}
	}
	for _, g := range []string{"C", "F"} {
		got := contributableMessage(g, RawMetrics{})
		if !strings.HasPrefix(got, "Unproven:") || strings.Contains(got, "never merged") {
			t.Errorf("grade %s with no sample must not claim rejection: %q", g, got)
		}
	}
}

func TestReplyTimeAndAgo(t *testing.T) {
	cases := []struct {
		hours float64
		want  string
	}{
		{0, ""}, {0.5, "first reply within the hour"}, {27, "first reply in about 27h"}, {96, "first reply in about 4d"},
	}
	for _, c := range cases {
		if got := replyTime(c.hours); got != c.want {
			t.Errorf("replyTime(%v) = %q, want %q", c.hours, got, c.want)
		}
	}
	for days, want := range map[int]string{0: "today", 10: "10d ago", 130: "4mo ago", 800: "2.2y ago"} {
		if got := agoDays(days); got != want {
			t.Errorf("agoDays(%d) = %q, want %q", days, got, want)
		}
	}
}

func TestVerdictMatrix(t *testing.T) {
	raw := activeRaw()
	q := func(g string) QuestionScore { return QuestionScore{Grade: g} }
	cats := []CategoryScore{
		{Key: CategoryActivity, Subs: []SubScore{{Label: "Commit recency", Value: 90}, {Label: "Issue close ratio", Value: 20, Raw: "40/200 issues closed (90d)"}}},
		{Key: CategoryCommunity, Subs: []SubScore{{Label: "License", Value: 100}, {Label: "Newcomer merge rate", Value: 11, Raw: "1/9 newcomer PRs merged"}}},
	}
	cases := []struct {
		m, c  string
		gates []Gate
		want  string
	}{
		{"A", "B", nil, "Worth your time"},
		{"C", "A", nil, "Your PR will likely land, but issue close ratio drags the outlook (40/200 issues closed (90d))."},
		{"B", "D", nil, "Alive and shipping, but newcomer merge rate drags newcomers (1/9 newcomer PRs merged); open an issue first."},
		{"C", "C", nil, "Coin flip"},
		{"D", "D", nil, "Weak on both counts"},
		{"F", "A", nil, "Look elsewhere unless you have a specific reason: the project looks dormant"},
		{"A", "F", nil, "Look elsewhere unless you have a specific reason: outside PRs are almost never merged"},
		{"A", "A", []Gate{{Severity: SeverityCritical, Title: "Archived or disabled"}}, "Look elsewhere unless you have a specific reason: archived or disabled"},
	}
	for _, c := range cases {
		got := buildVerdict(q(c.m), q(c.c), c.gates, cats, raw)
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("%s/%s: %q, want prefix %q", c.m, c.c, got, c.want)
		}
		if !strings.Contains(got, "Last push 3d ago, 9 of 10 newcomer PRs merged, first reply in about 8h.") {
			t.Errorf("%s/%s: evidence clause missing: %q", c.m, c.c, got)
		}
	}
}

func TestEvaluateProducesDecisionSentences(t *testing.T) {
	r := Evaluate(activeRaw())
	if !strings.Contains(r.Verdict, "Worth your time") {
		t.Errorf("verdict = %q", r.Verdict)
	}
	if !strings.Contains(r.Maintained.Message, "commits a week") || !strings.Contains(r.Contributable.Message, "newcomer PRs merged") {
		t.Errorf("question messages = %q / %q", r.Maintained.Message, r.Contributable.Message)
	}
	if got := QuestionVerdicts(r); got[0].Message != r.Maintained.Message {
		t.Error("QuestionVerdicts should return the report's own verdicts")
	}
}

func TestGateDetailsCarryNumbers(t *testing.T) {
	raw := RawMetrics{TopContributorRecentShare: 0.85, ContributorCount: 2}
	g, ok := busFactorGate(raw)
	if !ok || !strings.Contains(g.Detail, "85.0%") || !strings.Contains(g.Detail, "only 2 people") {
		t.Errorf("bus factor detail = %q", g.Detail)
	}
	raw = RawMetrics{NewcomerPRsMerged: 1, NewcomerPRsClosedUnmerged: 8}
	g, ok = closedToStrangersGate(raw, subLookup{prAcceptance: 80, newcomerMergeRate: 11})
	if !ok || !strings.Contains(g.Detail, "1 of 9 newcomer PRs") {
		t.Errorf("closed-to-strangers detail = %q", g.Detail)
	}
	g, ok = staleOrArchivedGate(RawMetrics{DaysSinceLastPush: 430}, subLookup{})
	if !ok || !strings.Contains(g.Detail, "No commits in 14 months") {
		t.Errorf("stale detail = %q", g.Detail)
	}
	g, ok = vanityStarsGate(RawMetrics{Stars: 20000, Watchers: 8})
	if !ok || !strings.Contains(g.Detail, "20.0k stars, 8 watchers") {
		t.Errorf("vanity detail = %q", g.Detail)
	}
	g, _ = staleOrArchivedGate(RawMetrics{Archived: true}, subLookup{})
	if strings.Contains(g.HowToClear, "nformational") {
		t.Errorf("filler how-to-clear survived: %q", g.HowToClear)
	}
}

func TestRawReadingsReadAsQuietNotBroken(t *testing.T) {
	if got := issueCloseRatio(RawMetrics{}).Raw; got != "no issues in 90d" {
		t.Errorf("issue reading = %q", got)
	}
	if got := commitFrequency(RawMetrics{CommitsLast52Weeks: make([]int, 52)}).Raw; got != "no commits most weeks (12wk median)" {
		t.Errorf("commit reading = %q", got)
	}
	if got := prResponsiveness(RawMetrics{OpenPRCount: 3, MedianOpenPRAgeDays: 124, StaleNewcomerOpenPRs: 12}).Raw; got != "PRs sit 124d; 12 newcomer PRs stale" {
		t.Errorf("pr reading = %q", got)
	}
	if got := prResponsiveness(RawMetrics{OpenPRCount: 3, MedianOpenPRAgeDays: 12}).Raw; got != "PRs sit 12d; none stale" {
		t.Errorf("pr reading = %q", got)
	}
	if got := prResponsiveness(RawMetrics{OpenPRCount: 3, MedianOpenPRAgeDays: 12, StaleNewcomerOpenPRs: 1}).Raw; got != "PRs sit 12d; 1 newcomer PR stale" {
		t.Errorf("singular pr reading = %q", got)
	}
	for _, c := range []struct{ got, want string }{
		{prBacklog(RawMetrics{}).Raw, "no PRs in 90d"},
		{prAcceptance(RawMetrics{}).Raw, "no PRs closed yet"},
		{newcomerMergeRate(RawMetrics{}).Raw, "no newcomer PRs closed yet"},
	} {
		if c.got != c.want {
			t.Errorf("reading = %q, want %q", c.got, c.want)
		}
	}
	g, _ := busFactorGate(RawMetrics{TopContributorRecentShare: 0.9, ContributorCount: 1})
	if !strings.Contains(g.Detail, "only 1 person contributes") {
		t.Errorf("singular bus factor = %q", g.Detail)
	}
	if got := QuestionVerdicts(Report{Categories: Evaluate(activeRaw()).Categories})[0].Message; got != "" {
		t.Errorf("fallback verdicts must not invent readings: %q", got)
	}
}

func TestCommitPaceBuckets(t *testing.T) {
	weeks := func(v int) []int {
		w := make([]int, 52)
		for i := range w {
			w[i] = v
		}
		return w
	}
	cases := []struct {
		raw  RawMetrics
		want string
	}{
		{RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 0.4}, "under 1 default-branch commit a week"},
		{RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 1.2}, "1 default-branch commit a week"},
		{RawMetrics{CommitsLast52Weeks: weeks(7)}, "7 default-branch commits a week"},
		{RawMetrics{}, ""},
	}
	for _, c := range cases {
		if got := commitPace(c.raw); got != c.want {
			t.Errorf("commitPace(%+v) = %q, want %q", c.raw, got, c.want)
		}
	}
	if got := spanDays(800); got != "2.2 years" {
		t.Errorf("spanDays(800) = %q", got)
	}
}
