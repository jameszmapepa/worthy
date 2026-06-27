package metrics

import (
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// Sinks prevent the compiler from eliding the calls under test in the
// allocation probes below.
var (
	sinkInt int
	sinkF64 float64
)

// benchOpenPulls builds n open PRs of mixed ages, all by newcomers, so
// processOpenPulls does real work (age median + stale-newcomer counting).
func benchOpenPulls(n int, now time.Time) []github.PullRequest {
	prs := make([]github.PullRequest, n)
	for i := range prs {
		prs[i] = github.PullRequest{
			Number:      i + 1,
			State:       "open",
			CreatedAt:   now.Add(-time.Duration(i+1) * 24 * time.Hour),
			User:        github.User{Login: "user", Type: "User"},
			AuthorAssoc: "FIRST_TIME_CONTRIBUTOR",
		}
	}
	return prs
}

func BenchmarkProcessOpenPulls(b *testing.B) {
	prs := benchOpenPulls(100, benchNow)
	b.ReportAllocs()
	for b.Loop() {
		processOpenPulls(prs, benchNow)
	}
}

// TestPerf_ProcessPullsZeroAlloc pins the zero-allocation property of the
// closed-PR outcome scan: it only counts over an existing slice and must never
// start allocating (a regression would show up here, not just in a benchmark
// nobody reads).
func TestPerf_ProcessPullsZeroAlloc(t *testing.T) {
	prs := benchPulls(100, benchNow)
	got := testing.AllocsPerRun(100, func() {
		sinkInt, _, _, _ = processPulls(prs, benchNow)
	})
	if got != 0 {
		t.Errorf("processPulls allocates %.0f times/run; want 0 (must stay allocation-free)", got)
	}
}

// TestPerf_BusFactorZeroAlloc pins the zero-allocation property of the bus
// factor computation over contributor stats.
func TestPerf_BusFactorZeroAlloc(t *testing.T) {
	stats := benchContributorStats(50)
	got := testing.AllocsPerRun(100, func() {
		sinkF64, sinkInt = busFactor(stats)
	})
	if got != 0 {
		t.Errorf("busFactor allocates %.0f times/run; want 0 (must stay allocation-free)", got)
	}
}
