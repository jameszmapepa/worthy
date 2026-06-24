package score

import (
	"math"
	"testing"
)

const eps = 1e-6

func approx(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// findSub locates a sub-score by key across all categories in a Report.
func findSub(t *testing.T, r Report, key string) SubScore {
	t.Helper()
	for _, c := range r.Categories {
		for _, s := range c.Subs {
			if s.Key == key {
				return s
			}
		}
	}
	t.Fatalf("sub-score %q not found in report", key)
	return SubScore{}
}

func TestCommitFrequency(t *testing.T) {
	tests := []struct {
		name string
		raw  RawMetrics
		want float64
	}{
		{"empty series is no-data -> neutral 50", RawMetrics{CommitsLast52Weeks: nil}, 50},
		{"present but all zero is genuinely inactive -> 0", RawMetrics{CommitsLast52Weeks: zeros(12)}, 0},
		{"median 15 saturates -> 100", RawMetrics{CommitsLast52Weeks: repeat(15, 12)}, 100},
		{"median 30 clamped -> 100", RawMetrics{CommitsLast52Weeks: repeat(30, 12)}, 100},
		{"even count median is mean of middle two", RawMetrics{CommitsLast52Weeks: repeat(7, 8, 7, 8)}, 100.0 * 7.5 / 15},
		{"odd count median is middle value", RawMetrics{CommitsLast52Weeks: repeat(3, 9, 6)}, 100.0 * 6 / 15},
		{"uses last 12 weeks only", RawMetrics{CommitsLast52Weeks: append(repeat(0, 40), repeat(15, 12)...)}, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := commitFrequency(tc.raw)
			approx(t, got.Value, tc.want, "commit_frequency")
		})
	}
}

func TestMedianLast(t *testing.T) {
	// medianLast guards empty input defensively even though commitFrequency now
	// short-circuits before reaching it; assert the guard directly.
	if got := medianLast(nil, 12); got != 0 {
		t.Errorf("medianLast(nil) = %v, want 0", got)
	}
	if got := medianLast([]int{5}, 12); got != 5 {
		t.Errorf("medianLast([5]) = %v, want 5", got)
	}
	if got := medianLast([]int{1, 2, 3, 4}, 2); got != 3.5 {
		t.Errorf("medianLast last-2 of [1,2,3,4] = %v, want 3.5", got)
	}
}

func TestCommitRecency(t *testing.T) {
	tests := []struct {
		days int
		want float64
	}{
		{0, 100},
		{365, 0},
		{730, 0}, // clamped, never negative
		{182, 100 - 182.0/365*100},
	}
	for _, tc := range tests {
		got := commitRecency(RawMetrics{DaysSinceLastPush: tc.days})
		approx(t, got.Value, tc.want, "commit_recency")
	}
}

func TestReleaseCadence(t *testing.T) {
	tests := []struct {
		name  string
		count int
		days  int
		want  float64
	}{
		{"no releases -> 40", 0, 0, 40},
		{"recent <=90d -> 100", 3, 30, 100},
		{"boundary 90d -> 100", 3, 90, 100},
		{"boundary 730d -> 0", 3, 730, 0},
		{"older than 730d -> 0", 3, 1000, 0},
		{"midpoint 410d", 3, 410, 100 * (730.0 - 410) / (730 - 90)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := releaseCadence(RawMetrics{ReleaseCount: tc.count, DaysSinceLastRelease: tc.days})
			approx(t, got.Value, tc.want, "release_cadence")
		})
	}
}

func TestIssueCloseRatio(t *testing.T) {
	tests := []struct {
		name         string
		closed, open int
		want         float64
	}{
		{"no issues -> 50 (neutral)", 0, 0, 50},
		{"all closed -> 100", 10, 0, 100},
		{"all open -> 0", 0, 10, 0},
		{"half -> 50", 5, 5, 50},
		// Cohort-specific: realistic 90d numbers.
		{"8 closed / 2 open -> 80", 8, 2, 80},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := issueCloseRatio(RawMetrics{RecentIssuesClosed: tc.closed, RecentIssuesOpen: tc.open})
			approx(t, got.Value, tc.want, "issue_close_ratio")
		})
	}
}

func TestPRBacklog(t *testing.T) {
	tests := []struct {
		name         string
		merged, open int
		want         float64
	}{
		{"no PRs -> 50 (neutral)", 0, 0, 50},
		{"all merged -> 100", 10, 0, 100},
		{"all open -> 0", 0, 10, 0},
		{"half -> 50", 5, 5, 50},
		// Cohort-specific: realistic 90d numbers.
		{"6 merged / 4 open -> 60", 6, 4, 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := prBacklog(RawMetrics{RecentPRsMerged: tc.merged, RecentPRsOpen: tc.open})
			approx(t, got.Value, tc.want, "pr_backlog")
		})
	}
}

func TestIssueResponsiveness(t *testing.T) {
	tests := []struct {
		name  string
		hours float64
		want  float64
	}{
		{"no data (0) -> 50", 0, 50},
		{"<=24h -> 100", 12, 100},
		{"boundary 24h -> 100", 24, 100},
		{"boundary 168h -> 60", 168, 60},
		{"mid 24-168", 96, 100 - (96.0-24)/(168-24)*40},
		{"boundary 720h -> 0", 720, 0},
		{"mid 168-720", 444, 60 - (444.0-168)/(720-168)*60},
		{">720h -> 0", 1000, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := issueResponsiveness(RawMetrics{MedianIssueFirstResponseHours: tc.hours})
			approx(t, got.Value, tc.want, "issue_responsiveness")
		})
	}
}

func TestPRAcceptance(t *testing.T) {
	tests := []struct {
		name                   string
		merged, closedUnmerged int
		want                   float64
	}{
		{"no closed PRs -> 50", 0, 0, 50},
		{"all merged -> 100", 10, 0, 100},
		{"all rejected -> 0", 0, 10, 0},
		{"half -> 50", 5, 5, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := prAcceptance(RawMetrics{MergedPRs: tc.merged, ClosedUnmergedPRs: tc.closedUnmerged})
			approx(t, got.Value, tc.want, "pr_acceptance")
		})
	}
}

func TestNewcomerMergeRate(t *testing.T) {
	tests := []struct {
		name             string
		merged, unmerged int
		want             float64
	}{
		{"no newcomer PRs -> 50", 0, 0, 50},
		{"all merged -> 100", 4, 0, 100},
		{"all closed -> 0", 0, 4, 0},
		{"half -> 50", 2, 2, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newcomerMergeRate(RawMetrics{
				NewcomerPRsMerged:         tc.merged,
				NewcomerPRsClosedUnmerged: tc.unmerged,
			})
			approx(t, got.Value, tc.want, "newcomer_merge_rate")
		})
	}
}

func TestGovernanceDocs(t *testing.T) {
	tests := []struct {
		name                      string
		readme, contrib, coc, lic bool
		want                      float64
	}{
		{"none -> 0", false, false, false, false, 0},
		{"all -> 100", true, true, true, true, 100},
		{"readme only -> 25", true, false, false, false, 25},
		{"license only -> 30", false, false, false, true, 30},
		{"coc only -> 20", false, false, true, false, 20},
		{"contributing only -> 25", false, true, false, false, 25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := governanceDocs(RawMetrics{
				HasReadme:        tc.readme,
				HasContributing:  tc.contrib,
				HasCodeOfConduct: tc.coc,
				HasLicense:       tc.lic,
			})
			approx(t, got.Value, tc.want, "governance_docs")
		})
	}
}

func TestLicense(t *testing.T) {
	tests := []struct {
		spdx string
		want float64
	}{
		{"MIT", 100},
		{"Apache-2.0", 100},
		{"", 0},
		{"NOASSERTION", 0},
	}
	for _, tc := range tests {
		got := licenseScore(RawMetrics{LicenseSPDX: tc.spdx})
		approx(t, got.Value, tc.want, "license:"+tc.spdx)
	}
}

func TestCIPresent(t *testing.T) {
	if got := ciPresent(RawMetrics{HasCI: true}); got.Value != 100 {
		t.Errorf("ci_present true = %v, want 100", got.Value)
	}
	if got := ciPresent(RawMetrics{HasCI: false}); got.Value != 0 {
		t.Errorf("ci_present false = %v, want 0", got.Value)
	}
}

func TestSignedReleases(t *testing.T) {
	tests := []struct {
		name   string
		signed bool
		count  int
		want   float64
	}{
		{"signed -> 100", true, 5, 100},
		{"no releases -> 40", false, 0, 40},
		{"unsigned releases -> 0", false, 5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := signedReleases(RawMetrics{HasSignedReleaseAssets: tc.signed, ReleaseCount: tc.count})
			approx(t, got.Value, tc.want, "signed_releases")
		})
	}
}

func TestSecurityPolicy(t *testing.T) {
	if got := securityPolicy(RawMetrics{HasSecurityPolicy: true}); got.Value != 100 {
		t.Errorf("security_policy true = %v, want 100", got.Value)
	}
	if got := securityPolicy(RawMetrics{HasSecurityPolicy: false}); got.Value != 0 {
		t.Errorf("security_policy false = %v, want 0", got.Value)
	}
}

func TestWorkflowSafety(t *testing.T) {
	tests := []struct {
		name       string
		usesTarget bool
		fetched    bool
		want       float64
	}{
		{"uses pull_request_target -> 30", true, true, 30},
		{"not fetched -> 70", false, false, 70},
		{"safe -> 100", false, true, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := workflowSafety(RawMetrics{
				UsesPullRequestTarget: tc.usesTarget,
				WorkflowsFetched:      tc.fetched,
			})
			approx(t, got.Value, tc.want, "workflow_safety")
		})
	}
}

// zeros returns a slice of n zeros.
func zeros(n int) []int { return make([]int, n) }

// repeat builds a slice by repeating the given pattern values; the final
// argument is the repeat count when a single value is given, otherwise the
// pattern is concatenated as-is. Two forms are supported:
//
//	repeat(v, n)          -> n copies of v
//	repeat(a, b, c, ...)  -> the literal sequence a, b, c, ...
//
// The two-arg form is the common saturation/boundary helper.
func repeat(vals ...int) []int {
	if len(vals) == 2 {
		out := make([]int, vals[1])
		for i := range out {
			out[i] = vals[0]
		}
		return out
	}
	out := make([]int, len(vals))
	copy(out, vals)
	return out
}
