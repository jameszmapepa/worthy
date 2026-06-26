package metrics

import (
	"testing"
	"time"

	"github.com/jameszmapepa/worthy/internal/github"
)

var (
	sinkInt int
	sinkF64 float64
)

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

func TestPerf_ProcessPullsZeroAlloc(t *testing.T) {
	prs := benchPulls(100, benchNow)
	got := testing.AllocsPerRun(100, func() {
		sinkInt, _, _, _ = processPulls(prs, benchNow)
	})
	if got != 0 {
		t.Errorf("processPulls allocates %.0f times/run; want 0 (must stay allocation-free)", got)
	}
}

func TestPerf_BusFactorZeroAlloc(t *testing.T) {
	stats := benchContributorStats(50)
	got := testing.AllocsPerRun(100, func() {
		sinkF64, sinkInt = busFactor(stats)
	})
	if got != 0 {
		t.Errorf("busFactor allocates %.0f times/run; want 0 (must stay allocation-free)", got)
	}
}
