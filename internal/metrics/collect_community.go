package metrics

import (
	"context"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/jameszmapepa/worthy/internal/github"
)

var newcomerAssocs = map[string]bool{
	"FIRST_TIME_CONTRIBUTOR": true,
	"NONE":                   true,
	"CONTRIBUTOR":            true,
}

const newcomerWindow = 90 * 24 * time.Hour

// ceiling: 12 issues × 1 API call each.
const issueSampleCap = 12

func processPulls(prs []github.PullRequest, now time.Time) (merged, unmerged, newcomerMerged, newcomerUnmerged int) {
	windowStart := now.Add(-newcomerWindow)
	for _, pr := range prs {
		if pr.IsMerged() {
			merged++
		} else {
			unmerged++
		}

		if !newcomerAssocs[pr.AuthorAssoc] {
			continue
		}

		if pr.ClosedAt == nil || pr.ClosedAt.Before(windowStart) {
			continue
		}

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

func isBot(u github.User) bool {
	return strings.HasSuffix(u.Login, "[bot]") || u.Type == "Bot"
}

// ceiling: samples up to issueSampleCap (12) issues with one API call each.
func medianTTFR(
	gctx context.Context,
	c *github.Client,
	owner, repo string,
	sem *semaphore.Weighted,
	issues []github.Issue,
) (float64, error) {
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
					return nil
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
	slices.Sort(hours)
	mid := len(hours) / 2
	if len(hours)%2 == 0 {
		return (hours[mid-1] + hours[mid]) / 2, nil
	}
	return hours[mid], nil
}

var staleOpenPRNewcomerAssocs = map[string]bool{
	"FIRST_TIME_CONTRIBUTOR": true,
	"FIRST_TIMER":            true,
	"NONE":                   true,
	"CONTRIBUTOR":            true,
}

const staleNewcomerOpenPRDays = 30

// ceiling: ≤100 PRs (one page); repos with >100 open PRs under-count stale newcomer PRs.
func processOpenPulls(prs []github.PullRequest, now time.Time) (openCount int, medianAgeDays float64, staleNewcomerCount int) {
	ages := make([]float64, 0, len(prs))
	for _, pr := range prs {
		if pr.State != "open" {
			continue
		}
		ageDays := now.Sub(pr.CreatedAt).Hours() / 24
		ages = append(ages, ageDays)
		openCount++
		if staleOpenPRNewcomerAssocs[pr.AuthorAssoc] && ageDays > staleNewcomerOpenPRDays {
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
