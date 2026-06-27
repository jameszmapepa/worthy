package score

import (
	"math"
	"strings"
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
		// Fallback: no stats series but a commits-count average is available.
		{"fallback avg scores like the series", RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 15}, 100},
		{"fallback avg scales linearly", RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 7.5}, 50},
		{"fallback avg zero -> 0", RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 0}, 0},
		// A present series wins over a fallback (priority order).
		{"series preferred over fallback", RawMetrics{CommitsLast52Weeks: repeat(15, 12), HasCommitFallback: true, CommitsPerWeekFallback: 0}, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := commitFrequency(tc.raw)
			approx(t, got.Value, tc.want, "commit_frequency")
		})
	}
}

func TestCommitFrequencyRawNotes(t *testing.T) {
	if got := commitFrequency(RawMetrics{}).Raw; got != "commit stats unavailable" {
		t.Errorf("no-data Raw = %q; want honest unavailable note", got)
	}
	if got := commitFrequency(RawMetrics{HasCommitFallback: true, CommitsPerWeekFallback: 4}).Raw; !strings.Contains(got, "12wk avg") {
		t.Errorf("fallback Raw = %q; want it to mention the 12wk avg", got)
	}
}

func TestNewcomerSignals(t *testing.T) {
	tests := []struct {
		name     string
		raw      RawMetrics
		wantVal  float64
		wantNote string
	}{
		{"unavailable -> neutral", RawMetrics{NewcomerLabelsAvailable: false}, newcomerSignalsNone, "label data unavailable"},
		{"available open door -> 100", RawMetrics{NewcomerLabelsAvailable: true, NewcomerLabeledOpen: 5, NewcomerLabeledAvailable: 2}, newcomerSignalsAvailable, "available beginner issues"},
		{"present but all claimed -> 60", RawMetrics{NewcomerLabelsAvailable: true, NewcomerLabeledOpen: 5, NewcomerLabeledAvailable: 0}, newcomerSignalsClaimed, "all assigned"},
		{"no labels at all -> neutral, not F", RawMetrics{NewcomerLabelsAvailable: true, NewcomerLabeledOpen: 0}, newcomerSignalsNone, "no beginner-labelled issues"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newcomerSignals(tc.raw)
			approx(t, got.Value, tc.wantVal, "newcomer_signals")
			if !strings.Contains(got.Raw, tc.wantNote) {
				t.Errorf("Raw = %q; want to contain %q", got.Raw, tc.wantNote)
			}
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
	// LICENSE is intentionally excluded from governance_docs (it is scored by the
	// standalone `license` sub-score) to avoid double-counting it in Community.
	// Remaining doc weights re-normalize to README .40 / CONTRIB .35 / CoC .25.
	tests := []struct {
		name                 string
		readme, contrib, coc bool
		want                 float64
	}{
		{"none -> 0", false, false, false, 0},
		{"all docs -> 100", true, true, true, 100},
		{"readme only -> 40", true, false, false, 40},
		{"coc only -> 25", false, false, true, 25},
		{"contributing only -> 35", false, true, false, 35},
		{"readme + coc -> 65 (additive, not overwritten)", true, false, true, 65},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := governanceDocs(RawMetrics{
				HasReadme:        tc.readme,
				HasContributing:  tc.contrib,
				HasCodeOfConduct: tc.coc,
			})
			approx(t, got.Value, tc.want, "governance_docs")
		})
	}
}

// TestBusFactor was deleted: the busFactor(raw RawMetrics) SubScore function was
// removed in B5. Gate behaviour is now covered by TestBusFactorGateCapsMaintenedQuestion
// in composite_test.go; the metrics-layer busFactor has its own tests in internal/metrics.

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

func TestPRResponsiveness(t *testing.T) {
	// B2 regression: prResponsiveness must return neutral 50 when there are no
	// open PRs (absent-signal convention), and must score based on median age
	// and stale-newcomer count when open PRs exist.
	tests := []struct {
		name        string
		openCount   int
		medianDays  float64
		staleNcomer int
		wantExact   *float64 // non-nil → exact match via approx
		wantMin     float64  // for range checks
		wantMax     float64
	}{
		{
			name:        "no open PRs -> neutral 50",
			openCount:   0,
			medianDays:  0,
			staleNcomer: 0,
			wantExact:   func() *float64 { v := 50.0; return &v }(),
		},
		{
			name:        "fresh PRs, no stale newcomers -> high score",
			openCount:   3,
			medianDays:  7,                                           // <= ageLo=14 -> freshness=100
			staleNcomer: 0,                                           // staleScore=100
			wantExact:   func() *float64 { v := 100.0; return &v }(), // 0.6*100+0.4*100=100
		},
		{
			name:        "very stale PRs, max stale newcomers -> low score",
			openCount:   5,
			medianDays:  180, // >= ageHi=180 -> freshness=0
			staleNcomer: 5,   // >= maxStale=5 -> staleScore=0
			wantExact:   func() *float64 { v := 0.0; return &v }(),
		},
		{
			name:        "mid-range: blended score",
			openCount:   4,
			medianDays:  97,   // linearDown(97,14,180) = 100*(1-(97-14)/166) ≈ 50
			staleNcomer: 2,    // (5-2)/5*100 = 60
			wantMin:     40.0, // 0.6*50 + 0.4*60 ≈ 54 — accept ±14 for float rounding
			wantMax:     68.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := RawMetrics{
				OpenPRCount:          tc.openCount,
				MedianOpenPRAgeDays:  tc.medianDays,
				StaleNewcomerOpenPRs: tc.staleNcomer,
			}
			got := prResponsiveness(raw)
			if got.Key != "pr_responsiveness" {
				t.Errorf("Key = %q, want pr_responsiveness", got.Key)
			}
			if tc.wantExact != nil {
				approx(t, got.Value, *tc.wantExact, "pr_responsiveness value")
			} else {
				if got.Value < tc.wantMin || got.Value > tc.wantMax {
					t.Errorf("pr_responsiveness value = %.2f; want [%.1f, %.1f]",
						got.Value, tc.wantMin, tc.wantMax)
				}
			}
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
