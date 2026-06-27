package metrics

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// newcomerAssocs is the set of author_association values that qualify a PR
// author as a newcomer for the newcomer-merge-rate metric.
var newcomerAssocs = map[string]bool{
	"FIRST_TIME_CONTRIBUTOR": true,
	"NONE":                   true,
	"CONTRIBUTOR":            true,
}

// newcomerWindow is the look-back window for newcomer PR classification.
const newcomerWindow = 90 * 24 * time.Hour

// issueSampleCap is the maximum number of real issues (non-PR) we fetch
// comments for when computing MedianIssueFirstResponseHours.
// ceiling: 12 issues × 1 API call each = 12 calls; adjust if budget allows.
const issueSampleCap = 12

// processPulls splits a closed-PR page into merged/unmerged counts and
// separately counts newcomer PRs (filtered by assoc, window, and self-merge).
func processPulls(prs []github.PullRequest, now time.Time) (merged, unmerged, newcomerMerged, newcomerUnmerged int) {
	windowStart := now.Add(-newcomerWindow)
	for _, pr := range prs {
		if pr.IsMerged() {
			merged++
		} else {
			unmerged++
		}

		// Newcomer classification.
		if !newcomerAssocs[pr.AuthorAssoc] {
			continue
		}
		// Must be within the 90-day window (use ClosedAt as the reference).
		if pr.ClosedAt == nil || pr.ClosedAt.Before(windowStart) {
			continue
		}
		// Exclude self-merges.
		if pr.IsMerged() && pr.MergedBy != nil && pr.MergedBy.Login == pr.User.Login {
			continue
		}
		if pr.IsMerged() {
			newcomerMerged++
		} else {
			newcomerUnmerged++
		}
	}
	return merged, unmerged, newcomerMerged, newcomerUnmerged
}

// isBot reports whether a github.User is a bot: login ends with "[bot]" or
// User.Type is "Bot".
func isBot(u github.User) bool {
	return strings.HasSuffix(u.Login, "[bot]") || u.Type == "Bot"
}

// medianTTFR computes the median time-to-first-response in hours across a
// sample of real (non-PR) issues. It skips issues where no qualifying
// response exists (first comment by someone other than the author, not a bot).
//
// ceiling: samples up to issueSampleCap (12) issues × 1 API call each;
// adjust cap if budget allows more calls.
//
// The per-issue comment fetches fan out under the shared semaphore and gctx.
// Non-context per-issue failures are swallowed (that issue is skipped); only
// a context cancellation/deadline is propagated, returned as the error so the
// caller can abort the whole collection.
func medianTTFR(
	gctx context.Context,
	c *github.Client,
	owner, repo string,
	sem *semaphore.Weighted,
	issues []github.Issue,
) (float64, error) {
	// Pick the sample of real (non-PR) issues that actually have at least one
	// comment, preserving order. The fetched issues are the newest-created; the
	// very newest are usually still unanswered (or only bot-triaged), so sampling
	// them blindly yielded "no issue response data" on active repos. Requiring
	// Comments>0 biases the sample toward issues that can have a first response
	// and avoids spending an API call per commentless issue.
	// ceiling: still drawn from the 100 newest-created issues; a repo whose only
	// answered issues are older than that window will under-sample. Widen the
	// RecentIssues fetch or add an updated-sorted fetch if that becomes an issue.
	sample := make([]github.Issue, 0, issueSampleCap)
	for _, iss := range issues {
		if iss.IsPullRequest() || iss.Comments == 0 {
			continue
		}
		if len(sample) >= issueSampleCap {
			break
		}
		sample = append(sample, iss)
	}

	// Indexed result slots; -1 marks "no qualifying response". Each goroutine
	// writes only its own index, so no synchronisation is needed.
	hoursByIssue := make([]float64, len(sample))
	for i := range hoursByIssue {
		hoursByIssue[i] = -1
	}

	g, ictx := errgroup.WithContext(gctx)
	for i, iss := range sample {
		g.Go(func() error {
			return withCall(ictx, sem, func() error {
				comments, err := c.IssueComments(ictx, owner, repo, iss.Number)
				if err != nil {
					if isContextError(err) {
						return err
					}
					return nil // swallow per-issue failure
				}
				for _, cm := range comments {
					if cm.User.Login == iss.User.Login || isBot(cm.User) {
						continue
					}
					h := cm.CreatedAt.Sub(iss.CreatedAt).Hours()
					if h >= 0 {
						hoursByIssue[i] = h
					}
					break
				}
				return nil
			})
		})
	}
	if err := g.Wait(); err != nil {
		return 0, err
	}

	hours := make([]float64, 0, len(sample))
	for _, h := range hoursByIssue {
		if h >= 0 {
			hours = append(hours, h)
		}
	}
	if len(hours) == 0 {
		return 0, nil
	}
	slices.Sort(hours) // A10: slices.Sort replaces sort.Float64s (stdlib, 1.21+)
	mid := len(hours) / 2
	if len(hours)%2 == 0 {
		return (hours[mid-1] + hours[mid]) / 2, nil
	}
	return hours[mid], nil
}

// staleOpenPRNewcomerAssocs is the set of author_association values that
// qualify an open-PR author as a newcomer for the stale-open-PR metric.
// FIRST_TIMER is GitHub's label for a user's very first PR on any repository.
// CONTRIBUTOR is treated as a newcomer proxy — distinguishing a CONTRIBUTOR
// with no prior merged PR from one with merged PRs requires per-user history
// queries outside our API budget.
var staleOpenPRNewcomerAssocs = map[string]bool{
	"FIRST_TIME_CONTRIBUTOR": true,
	"FIRST_TIMER":            true,
	"NONE":                   true,
	"CONTRIBUTOR":            true,
}

// staleThreshold is the age beyond which an open newcomer PR is considered
// ghosted (opened but not yet acknowledged by a maintainer).
const staleThreshold = 30 * 24 * time.Hour

// processOpenPulls computes the three open-PR ghosting metrics from a page of
// open PRs. It is intentionally defensive: the API may return non-open items
// when state filtering misbehaves, so each PR is checked for State=="open".
// ceiling: median and stale count are derived from ≤100 PRs (one API page);
// repos with >100 open PRs will under-count stale newcomer PRs.
func processOpenPulls(prs []github.PullRequest, now time.Time) (openCount int, medianAgeDays float64, staleNewcomerCount int) {
	ages := make([]float64, 0, len(prs))
	for _, pr := range prs {
		if pr.State != "open" {
			continue
		}
		ageDays := now.Sub(pr.CreatedAt).Hours() / 24
		ages = append(ages, ageDays)
		openCount++
		if staleOpenPRNewcomerAssocs[pr.AuthorAssoc] && ageDays > 30 {
			staleNewcomerCount++
		}
	}
	if len(ages) == 0 {
		return 0, 0, 0
	}
	slices.Sort(ages)
	mid := len(ages) / 2
	if len(ages)%2 == 0 {
		medianAgeDays = (ages[mid-1] + ages[mid]) / 2
	} else {
		medianAgeDays = ages[mid]
	}
	return openCount, medianAgeDays, staleNewcomerCount
}
