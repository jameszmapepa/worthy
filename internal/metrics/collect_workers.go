package metrics

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/jameszmapepa/worthy/internal/github"
)

type communityResult struct {
	ok                bool
	healthPercentage  int
	hasReadme         bool
	hasContributing   bool
	hasCodeOfConduct  bool
	hasSecurityPolicy bool
	partial           string
}

type contributorResult struct {
	topShare float64
	count    int
	partial  string
}

type commitResult struct {
	weekly []int
	// hasFallback distinguishes a real 0 from "no fallback data" when weekly is empty.
	weeklyAvg   float64
	hasFallback bool
	partial     string
}

type newcomerLabelResult struct {
	open      int
	available int
	ok        bool
	partial   string
}

type releaseResult struct {
	count     int
	daysSince int
	signed    bool
	partial   string
}

type workflowResult struct {
	hasCI   bool
	usesPRT bool
	fetched bool
	partial string
}

type closedPullsResult struct {
	merged           int
	unmerged         int
	newcomerMerged   int
	newcomerUnmerged int
	partial          string
}

type ttfrResult struct {
	median             float64
	recentIssuesClosed int
	recentIssuesOpen   int
	partial            string
}

type openPullsResult struct {
	openCount          int
	medianAgeDays      float64
	staleNewcomerCount int
	partial            string
}

type prCohortResult struct {
	recentPRsMerged int
	recentPRsOpen   int
	partial         string
}

func withCall(gctx context.Context, sem *semaphore.Weighted, fn func() error) error {
	if err := sem.Acquire(gctx, 1); err != nil {
		return err
	}
	defer sem.Release(1)
	return fn()
}

func collectCommunity(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *communityResult) error {
	var profile *github.CommunityProfile
	err := withCall(gctx, sem, func() error {
		p, e := c.CommunityProfile(gctx, owner, repo)
		profile = p
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "community_profile"
		return nil
	}
	out.ok = true
	out.healthPercentage = profile.HealthPercentage
	out.hasReadme = profile.Files.Readme != nil
	out.hasContributing = profile.Files.Contributing != nil
	out.hasCodeOfConduct = profile.Files.CodeOfConduct != nil
	out.hasSecurityPolicy = profile.Files.SecurityPol != nil
	return nil
}

func collectContributors(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *contributorResult) error {
	var stats []github.ContributorStats
	err := withCall(gctx, sem, func() error {
		s, e := c.ContributorStats(gctx, owner, repo)
		stats = s
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "contributor_stats"
		return nil
	}
	out.topShare, out.count = busFactor(stats)
	return nil
}

const commitFallbackWeeks = 12

func collectCommits(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *commitResult) error {
	var weeks []github.CommitActivityWeek
	statsErr := withCall(gctx, sem, func() error {
		w, e := c.CommitActivity(gctx, owner, repo)
		weeks = w
		return e
	})
	if statsErr != nil && isContextError(statsErr) {
		return statsErr
	}
	if statsErr == nil && len(weeks) > 0 {
		out.weekly = make([]int, len(weeks))
		for i, w := range weeks {
			out.weekly[i] = w.Total
		}
		return nil
	}

	since := now.Add(-commitFallbackWeeks * 7 * 24 * time.Hour)
	var count int
	fbErr := withCall(gctx, sem, func() error {
		n, e := c.CommitCountSince(gctx, owner, repo, since)
		count = n
		return e
	})
	if fbErr != nil {
		if isContextError(fbErr) {
			return fbErr
		}
		out.partial = "commit_activity"
		return nil
	}
	out.weeklyAvg = float64(count) / commitFallbackWeeks
	out.hasFallback = true
	return nil
}

// collectNewcomerLabels uses the Search API, which has a separate rate-limit budget.
func collectNewcomerLabels(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *newcomerLabelResult) error {
	base := fmt.Sprintf(
		`repo:%s/%s is:issue is:open label:"good first issue","good-first-issue","help wanted","help-wanted"`,
		owner, repo)

	var open int
	openErr := withCall(gctx, sem, func() error {
		n, e := c.SearchIssueCount(gctx, base)
		open = n
		return e
	})
	if openErr != nil {
		if isContextError(openErr) {
			return openErr
		}
		out.partial = "newcomer_labels"
		return nil
	}

	var available int
	availErr := withCall(gctx, sem, func() error {
		n, e := c.SearchIssueCount(gctx, base+" no:assignee")
		available = n
		return e
	})
	if availErr != nil {
		if isContextError(availErr) {
			return availErr
		}

		out.partial = "newcomer_labels"
		out.open = open
		return nil
	}
	out.open = open
	out.available = available
	out.ok = true
	return nil
}

func collectReleases(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *releaseResult) error {
	var releases []github.Release

	err := withCall(gctx, sem, func() error {
		r, e := c.Releases(gctx, owner, repo, 100)
		releases = r
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "releases"
		return nil
	}
	out.count, out.daysSince, out.signed = processReleases(releases, now)
	return nil
}

func collectWorkflows(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *workflowResult) error {
	var workflows []github.Workflow
	err := withCall(gctx, sem, func() error {
		w, e := c.Workflows(gctx, owner, repo)
		workflows = w
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "workflows"
		return nil
	}
	hasCI, usesPRT, fetched, needsPartial, err := processWorkflows(gctx, c, owner, repo, sem, workflows)
	if err != nil {
		return err
	}
	out.hasCI = hasCI
	out.usesPRT = usesPRT
	out.fetched = fetched
	if needsPartial {
		out.partial = "workflow_safety"
	}
	return nil
}

func collectClosedPulls(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *closedPullsResult) error {
	var pulls []github.PullRequest
	err := withCall(gctx, sem, func() error {
		p, e := c.RecentPulls(gctx, owner, repo, "closed")
		pulls = p
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "closed_pulls"
		return nil
	}
	out.merged, out.unmerged, out.newcomerMerged, out.newcomerUnmerged = processPulls(pulls, now)
	return nil
}

func collectTTFR(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *ttfrResult) error {
	var issues []github.Issue
	err := withCall(gctx, sem, func() error {
		is, e := c.RecentIssues(gctx, owner, repo, "all")
		issues = is
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "issue_ttfr"
		return nil
	}
	median, err := medianTTFR(gctx, c, owner, repo, sem, issues)
	if err != nil {
		return err
	}
	out.median = median

	out.recentIssuesClosed, out.recentIssuesOpen = issueCreationCohort(issues, now)
	return nil
}

func collectPRCohort(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *prCohortResult) error {
	var prs []github.PullRequest
	err := withCall(gctx, sem, func() error {
		p, e := c.RecentPullsByCreation(gctx, owner, repo)
		prs = p
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "pr_cohort"
		return nil
	}
	out.recentPRsMerged, out.recentPRsOpen = prCreationCohort(prs, now)
	return nil
}

func issueCreationCohort(issues []github.Issue, now time.Time) (closed, open int) {
	windowStart := now.Add(-newcomerWindow)
	for _, iss := range issues {
		if iss.IsPullRequest() {
			continue
		}
		if iss.CreatedAt.Before(windowStart) {
			continue
		}
		if iss.State == "closed" {
			closed++
		} else {
			open++
		}
	}
	return closed, open
}

func prCreationCohort(prs []github.PullRequest, now time.Time) (merged, open int) {
	windowStart := now.Add(-newcomerWindow)
	for _, pr := range prs {
		if pr.CreatedAt.Before(windowStart) {
			continue
		}
		switch {
		case pr.MergedAt != nil:
			merged++
		case pr.State == "open":
			open++

		}
	}
	return merged, open
}

// ceiling: ≤100 open PRs (one page); signal is directionally correct for larger repos.
func collectOpenPulls(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *openPullsResult) error {
	var prs []github.PullRequest
	err := withCall(gctx, sem, func() error {
		p, e := c.RecentPulls(gctx, owner, repo, "open")
		prs = p
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "open_pulls"
		return nil
	}
	out.openCount, out.medianAgeDays, out.staleNewcomerCount = processOpenPulls(prs, now)
	return nil
}
