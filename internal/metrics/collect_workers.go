package metrics

import (
	"context"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// Each collector writes exactly one of these result structs (its own), so the
// orchestrator can run them concurrently without any shared mutable state.
// The partial field carries the degradation marker (empty when the call
// succeeded), recorded by the owning collector only.

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
	weekly  []int
	partial string
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
	// A3: derived from the same issues slice — zero extra API calls.
	goodFirstIssues  int
	helpWantedIssues int
	partial          string
}

// openPullsResult carries the A2 open-PR ghosting metrics.
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

// withCall acquires one semaphore slot, runs fn, then releases the slot. It
// returns the context error from Acquire (which is gctx.Err() on cancellation)
// or whatever fn returns. The slot is held only for the duration of fn, never
// while spawning nested fan-outs.
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

func collectCommits(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *commitResult) error {
	var weeks []github.CommitActivityWeek
	err := withCall(gctx, sem, func() error {
		w, e := c.CommitActivity(gctx, owner, repo)
		weeks = w
		return e
	})
	if err != nil {
		if isContextError(err) {
			return err
		}
		out.partial = "commit_activity"
		return nil
	}
	out.weekly = make([]int, len(weeks))
	for i, w := range weeks {
		out.weekly[i] = w.Total
	}
	return nil
}

func collectReleases(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, now time.Time, out *releaseResult) error {
	var releases []github.Release
	// ceiling: fetch up to 100 releases; increase if repos with >100 releases
	// need an accurate DaysSinceLastRelease.
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
		return err // context error only
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
		return err // context error only
	}
	out.median = median
	// Derive the 90-day issue creation cohort and newcomer label counts from
	// the same slice — no extra API calls.
	out.recentIssuesClosed, out.recentIssuesOpen = issueCreationCohort(issues, now)
	out.goodFirstIssues, out.helpWantedIssues = countLabels(issues)
	return nil
}

// collectPRCohort fetches all-state PRs sorted by creation date and counts
// the 90-day creation cohort: merged (MergedAt!=nil) and open (State=="open").
// Closed-unmerged PRs are excluded from both numerator and denominator.
// On any non-context error the partial marker "pr_cohort" is recorded and
// counts stay 0, yielding a neutral 50 via ratioScore.
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

// issueCreationCohort counts non-PR issues created within the newcomerWindow
// as closed or open. Issues outside the window are ignored.
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

// prCreationCohort counts PRs created within the newcomerWindow as merged or
// open. Closed-unmerged PRs are excluded (they are irrelevant to the backlog
// health signal).
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
			// closed-unmerged: excluded from cohort
		}
	}
	return merged, open
}

// collectOpenPulls fetches the first page (100) of open PRs and computes
// OpenPRCount, MedianOpenPRAgeDays, and StaleNewcomerOpenPRs. These three
// metrics expose the open-PR ghosting signal: maintainers who leave newcomer
// PRs open indefinitely are invisible when only closed PRs are sampled.
// ceiling: samples up to 100 open PRs (one page); repos with >100 open PRs
// will under-count stale newcomer PRs but the signal is directionally correct.
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
