package metrics

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jameszmapepa/worthy/internal/github"
)

var benchNow = time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

func BenchmarkCollect(b *testing.B) {
	srv := httptest.NewServer(fullRoutesHandler(benchNow, 0))
	defer srv.Close()
	c := client(srv)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Collect(context.Background(), c, "acme", "widget", benchNow); err != nil {
			b.Fatalf("Collect: %v", err)
		}
	}
}

func benchContributorStats(n int) []github.ContributorStats {
	stats := make([]github.ContributorStats, n)
	for i := range stats {
		weeks := make([]github.ContributorWeek, 52)
		for w := range weeks {
			weeks[w] = github.ContributorWeek{
				Week:    int64(1600000000 + w*604800),
				Commits: (i + w) % 7,
			}
		}
		stats[i] = github.ContributorStats{
			Total:  52,
			Author: github.User{Login: "user" + strconv.Itoa(i), Type: "User"},
			Weeks:  weeks,
		}
	}
	return stats
}

func BenchmarkBusFactor(b *testing.B) {
	stats := benchContributorStats(50)
	b.ReportAllocs()
	for b.Loop() {
		busFactor(stats)
	}
}

func benchPulls(n int, now time.Time) []github.PullRequest {
	prs := make([]github.PullRequest, n)
	closed := now.Add(-20 * 24 * time.Hour)
	for i := range prs {
		p := github.PullRequest{
			Number:      i + 1,
			State:       "closed",
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
			ClosedAt:    &closed,
			User:        github.User{Login: "user" + strconv.Itoa(i), Type: "User"},
			AuthorAssoc: "FIRST_TIME_CONTRIBUTOR",
		}
		if i%2 == 0 {
			merged := closed
			p.MergedAt = &merged
			p.MergedBy = &github.User{Login: "maintainer", Type: "User"}
		}
		prs[i] = p
	}
	return prs
}

func BenchmarkProcessPulls(b *testing.B) {
	prs := benchPulls(100, benchNow)
	b.ReportAllocs()
	for b.Loop() {
		processPulls(prs, benchNow)
	}
}
