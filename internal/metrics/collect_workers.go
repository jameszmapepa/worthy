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
	hasLicense        bool
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

type countResult struct {
	openIssues   int
	closedIssues int
	openPRs      int
	closedPRs    int
	partial      []string
}

type closedPullsResult struct {
	merged           int
	unmerged         int
	newcomerMerged   int
	newcomerUnmerged int
	partial          string
}

type ttfrResult struct {
	median  float64
	partial string
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
	out.hasLicense = profile.Files.License != nil
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

// collectCounts fetches open/closed issue counts and PR counts under the shared
// semaphore. Each sub-call records its own canonical partial marker on
// non-context failure; a context error aborts the whole collection.
func collectCounts(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *countResult) error {
	openIssues, err := countOne(gctx, sem, func() (int, error) {
		return c.CountByState(gctx, owner, repo, "issues", "open")
	})
	if isContextError(err) {
		return err
	}
	if err != nil {
		out.partial = append(out.partial, "issue_count_open")
	}
	out.openIssues = openIssues

	closedIssues, err := countOne(gctx, sem, func() (int, error) {
		return c.CountByState(gctx, owner, repo, "issues", "closed")
	})
	if isContextError(err) {
		return err
	}
	if err != nil {
		out.partial = append(out.partial, "issue_count_closed")
	}
	out.closedIssues = closedIssues

	var openPRs, closedPRs int
	err = withCall(gctx, sem, func() error {
		var e error
		openPRs, closedPRs, e = c.PullRequestCounts(gctx, owner, repo)
		return e
	})
	if isContextError(err) {
		return err
	}
	if err != nil {
		out.partial = append(out.partial, "pr_counts")
	}
	out.openPRs = openPRs
	out.closedPRs = closedPRs
	return nil
}

// countOne runs a single count call under the semaphore, returning its value
// and error (so the caller can classify context vs degradation).
func countOne(gctx context.Context, sem *semaphore.Weighted, fn func() (int, error)) (int, error) {
	var n int
	err := withCall(gctx, sem, func() error {
		var e error
		n, e = fn()
		return e
	})
	return n, err
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

func collectTTFR(gctx context.Context, c *github.Client, owner, repo string, sem *semaphore.Weighted, out *ttfrResult) error {
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
	return nil
}
