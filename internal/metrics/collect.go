// Package metrics collects raw repository signals from the GitHub API and
// assembles them into a score.RawMetrics value. It performs no scoring — that
// is the score package's job — keeping data collection and judgement separate.
//
// The collector is split by domain for readability: this file owns the Collect
// orchestrator and its cross-cutting helpers, while the per-domain extraction
// logic lives in collect_activity.go, collect_community.go, and
// collect_security.go.
package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/score"
)

// Collect gathers every signal the scoring engine needs for owner/repo, using
// the supplied client. Time-relative metrics are computed against now (injected
// for deterministic tests).
//
// Error handling:
//   - Context cancellation or deadline exceeded on any endpoint aborts the
//     whole collection and returns the context error immediately.
//   - Rate-limit, not-found, or other transient errors on a non-fatal endpoint
//     are recorded by name in RawMetrics.Partial and collection continues.
//   - A non-nil error is returned ONLY when the core repository call fails or
//     the context is cancelled, so the caller can surface a useful message.
//
// CONTRACT: this signature is fixed; the TUI and cmd wire against it.
func Collect(ctx context.Context, c *github.Client, owner, repo string, now time.Time) (score.RawMetrics, error) {
	var raw score.RawMetrics

	// ------------------------------------------------------------------ //
	// 1. Core repository — fatal if missing, rate-limited, or cancelled.  //
	// ------------------------------------------------------------------ //
	repoData, err := c.Repository(ctx, owner, repo)
	if err != nil {
		return score.RawMetrics{}, err
	}
	raw.Stars = repoData.Stargazers
	raw.Watchers = repoData.Watchers
	raw.Forks = repoData.Forks
	raw.Archived = repoData.Archived
	raw.Disabled = repoData.Disabled
	raw.Description = repoData.Description
	raw.Language = repoData.Language
	raw.DaysSinceLastPush = int(now.Sub(repoData.PushedAt).Hours() / 24)
	raw.RepoAgeDays = int(now.Sub(repoData.CreatedAt).Hours() / 24)
	if repoData.License != nil {
		raw.LicenseSPDX = repoData.License.SPDXID
	}

	// ------------------------------------------------------------------ //
	// 2. Community profile — 404 on forks; degrade gracefully.           //
	// ------------------------------------------------------------------ //
	community, err := c.CommunityProfile(ctx, owner, repo)
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "community_profile")
	} else {
		raw.HealthPercentage = community.HealthPercentage
		raw.HasReadme = community.Files.Readme != nil
		raw.HasContributing = community.Files.Contributing != nil
		raw.HasLicense = community.Files.License != nil
		raw.HasCodeOfConduct = community.Files.CodeOfConduct != nil
		raw.HasSecurityPolicy = community.Files.SecurityPol != nil
	}

	// ------------------------------------------------------------------ //
	// 3. Contributor stats (bus factor).                                  //
	// ------------------------------------------------------------------ //
	stats, err := c.ContributorStats(ctx, owner, repo)
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "contributor_stats")
	} else {
		raw.TopContributorRecentShare, raw.ContributorCount = busFactor(stats)
	}

	// ------------------------------------------------------------------ //
	// 4. Commit activity (52-week series).                                //
	// ------------------------------------------------------------------ //
	weeks, err := c.CommitActivity(ctx, owner, repo)
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "commit_activity")
	} else {
		raw.CommitsLast52Weeks = make([]int, len(weeks))
		for i, w := range weeks {
			raw.CommitsLast52Weeks[i] = w.Total
		}
	}

	// ------------------------------------------------------------------ //
	// 5. Releases (exclude draft + prerelease).                           //
	// ceiling: fetch up to 100 releases (pageSize); increase if repos    //
	// with >100 releases need accurate DaysSinceLastRelease.             //
	// ------------------------------------------------------------------ //
	releases, err := c.Releases(ctx, owner, repo, 100)
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "releases")
	} else {
		raw.ReleaseCount, raw.DaysSinceLastRelease, raw.HasSignedReleaseAssets =
			processReleases(releases, now)
	}

	// ------------------------------------------------------------------ //
	// 6. Workflows: HasCI + UsesPullRequestTarget.                        //
	// ------------------------------------------------------------------ //
	workflows, err := c.Workflows(ctx, owner, repo)
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "workflows")
	} else {
		raw.HasCI, raw.UsesPullRequestTarget, raw.WorkflowsFetched =
			processWorkflows(ctx, c, owner, repo, workflows, &raw.Partial)
	}

	// ------------------------------------------------------------------ //
	// 7. Issue + PR counts.                                               //
	// ------------------------------------------------------------------ //
	openIssues, closedIssues, openPRs, closedPRs, partialCounts, ctxErr :=
		collectCounts(ctx, c, owner, repo)
	if ctxErr != nil {
		return raw, ctxErr
	}
	raw.Partial = append(raw.Partial, partialCounts...)
	raw.OpenPRs = openPRs

	// Subtract PRs from the /issues endpoint results (which includes PRs).
	raw.OpenIssues = clampZero(openIssues - openPRs)
	raw.ClosedIssues = clampZero(closedIssues - closedPRs)

	// ------------------------------------------------------------------ //
	// 8. Closed PRs: MergedPRs, ClosedUnmergedPRs, newcomer stats.       //
	// ------------------------------------------------------------------ //
	closedPullsList, err := c.RecentPulls(ctx, owner, repo, "closed")
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "closed_pulls")
	} else {
		raw.MergedPRs, raw.ClosedUnmergedPRs,
			raw.NewcomerPRsMerged, raw.NewcomerPRsClosedUnmerged =
			processPulls(closedPullsList, now)
	}

	// ------------------------------------------------------------------ //
	// 9. Median issue first-response hours (bot-filtered TTFR).          //
	// ------------------------------------------------------------------ //
	recentIssues, err := c.RecentIssues(ctx, owner, repo, "all")
	if err != nil {
		if isContextError(err) {
			return raw, err
		}
		raw.Partial = append(raw.Partial, "issue_ttfr")
	} else {
		raw.MedianIssueFirstResponseHours =
			medianTTFR(ctx, c, owner, repo, recentIssues)
	}

	return raw, nil
}

// isContextError reports whether err is a context cancellation or timeout.
// These abort the whole collection rather than degrading to Partial.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// collectCounts fetches open/closed issue counts and PR counts.
// It returns a context error (ctxErr) when the context is cancelled so the
// caller can abort; for all other errors it records the metric name in partial
// and continues.
func collectCounts(
	ctx context.Context,
	c *github.Client,
	owner, repo string,
) (openIssues, closedIssues, openPRs, closedPRs int, partial []string, ctxErr error) {
	var err error

	openIssues, err = c.CountByState(ctx, owner, repo, "issues", "open")
	if err != nil {
		if isContextError(err) {
			return 0, 0, 0, 0, nil, err
		}
		partial = append(partial, "issue_count_open")
	}

	closedIssues, err = c.CountByState(ctx, owner, repo, "issues", "closed")
	if err != nil {
		if isContextError(err) {
			return 0, 0, 0, 0, nil, err
		}
		partial = append(partial, "issue_count_closed")
	}

	openPRs, closedPRs, err = c.PullRequestCounts(ctx, owner, repo)
	if err != nil {
		if isContextError(err) {
			return 0, 0, 0, 0, nil, err
		}
		partial = append(partial, "pr_counts")
	}

	return openIssues, closedIssues, openPRs, closedPRs, partial, nil
}

// clampZero returns n if n >= 0, else 0.
func clampZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
