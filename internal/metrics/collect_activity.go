package metrics

import (
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// busFactor computes TopContributorRecentShare and ContributorCount from the
// last 12 weeks of each contributor's commit series.
// A7: single-pass over stats — the intermediate loginCommits slice stored login
// strings that were never read; collapsed to running grandTotal + topCommits
// accumulators. Behaviour is identical; the existing test suite guards this.
func busFactor(stats []github.ContributorStats) (topShare float64, count int) {
	var grandTotal, topCommits int
	for _, s := range stats {
		weeks := s.Weeks
		start := len(weeks) - 12
		if start < 0 {
			start = 0
		}
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

// processReleases filters out draft and prerelease entries, counts the
// remainder, computes DaysSinceLastRelease (from the most recent real
// release), and detects signed assets.
func processReleases(releases []github.Release, now time.Time) (count, daysSince int, signed bool) {
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		count++
		if r.PublishedAt != nil && count == 1 {
			// Most-recent real release (API returns newest first).
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
