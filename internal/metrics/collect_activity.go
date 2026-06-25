package metrics

import (
	"time"

	"github.com/jameszmapepa/worthy/internal/github"
)

func busFactor(stats []github.ContributorStats) (topShare float64, count int) {
	var grandTotal, topCommits int
	for _, s := range stats {
		weeks := s.Weeks
		start := max(len(weeks)-12, 0)
		total := 0
		for _, w := range weeks[start:] {
			total += w.Commits
		}
		if total == 0 {
			continue
		}
		count++
		grandTotal += total
		if total > topCommits {
			topCommits = total
		}
	}
	if count == 0 || grandTotal == 0 {
		return 0, count
	}
	return float64(topCommits) / float64(grandTotal), count
}

func processReleases(releases []github.Release, now time.Time) (count, daysSince int, signed bool) {
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		count++
		if r.PublishedAt != nil && count == 1 {
			daysSince = int(now.Sub(*r.PublishedAt).Hours() / 24)
		}
		if !signed {
			for _, a := range r.Assets {
				if hasSignatureExt(a.Name) {
					signed = true
					break
				}
			}
		}
	}
	return count, daysSince, signed
}
