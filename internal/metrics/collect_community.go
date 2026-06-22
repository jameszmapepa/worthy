package metrics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
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
func medianTTFR(
	ctx context.Context,
	c *github.Client,
	owner, repo string,
	issues []github.Issue,
) float64 {
	var hours []float64
	sampled := 0

	for _, iss := range issues {
		if iss.IsPullRequest() {
			continue
		}
		if sampled >= issueSampleCap {
			break
		}
		sampled++

		comments, err := c.IssueComments(ctx, owner, repo, iss.Number)
		if err != nil {
			continue
		}
		for _, cm := range comments {
			if cm.User.Login == iss.User.Login {
				continue // same author
			}
			if isBot(cm.User) {
				continue // bot
			}
			// First qualifying response found.
			h := cm.CreatedAt.Sub(iss.CreatedAt).Hours()
			if h >= 0 {
				hours = append(hours, h)
			}
			break
		}
	}

	if len(hours) == 0 {
		return 0
	}
	sort.Float64s(hours)
	mid := len(hours) / 2
	if len(hours)%2 == 0 {
		return (hours[mid-1] + hours[mid]) / 2
	}
	return hours[mid]
}
