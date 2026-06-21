// Package metrics collects raw repository signals from the GitHub API and
// assembles them into a score.RawMetrics value. It performs no scoring — that
// is the score package's job — keeping data collection and judgement separate.
package metrics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/score"
)

// newcomerAssocs is the set of author_association values that qualify a PR
// author as a newcomer for the newcomer-merge-rate metric.
var newcomerAssocs = map[string]bool{
	"FIRST_TIME_CONTRIBUTOR": true,
	"NONE":                   true,
	"CONTRIBUTOR":            true,
}

// signatureExts is the set of file-name suffixes that indicate a signed
// release asset per the spec.
var signatureExts = []string{".asc", ".sig", ".sigstore", ".intoto.jsonl"}

// newcomerWindow is the look-back window for newcomer PR classification.
const newcomerWindow = 90 * 24 * time.Hour

// issueSampleCap is the maximum number of real issues (non-PR) we fetch
// comments for when computing MedianIssueFirstResponseHours.
// ceiling: 12 issues × 1 API call each = 12 calls; adjust if budget allows.
const issueSampleCap = 12

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
	var ctxErr error
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

// ------------------------------------------------------------------ //
// Helpers                                                             //
// ------------------------------------------------------------------ //

// isContextError reports whether err is a context cancellation or timeout.
// These abort the whole collection rather than degrading to Partial.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// busFactor computes TopContributorRecentShare and ContributorCount from the
// last 12 weeks of each contributor's commit series.
func busFactor(stats []github.ContributorStats) (topShare float64, count int) {
	type loginCommits struct {
		login   string
		commits int
	}
	var contributors []loginCommits
	for _, s := range stats {
		weeks := s.Weeks
		// Take last 12 weeks; fewer weeks means fewer entries.
		start := len(weeks) - 12
		if start < 0 {
			start = 0
		}
		total := 0
		for _, w := range weeks[start:] {
			total += w.Commits
		}
		if total > 0 {
			contributors = append(contributors, loginCommits{s.Author.Login, total})
		}
	}
	count = len(contributors)
	if count == 0 {
		return 0, 0
	}
	grandTotal := 0
	topCommits := 0
	for _, lc := range contributors {
		grandTotal += lc.commits
		if lc.commits > topCommits {
			topCommits = lc.commits
		}
	}
	if grandTotal == 0 {
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

// hasSignatureExt reports whether name ends with a signature extension.
func hasSignatureExt(name string) bool {
	for _, ext := range signatureExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// processWorkflows determines HasCI, UsesPullRequestTarget, and
// WorkflowsFetched by fetching each workflow file's content and scanning for
// the literal "pull_request_target" string.
//
// Per-file tolerance: a fetch failure (404, rate-limit, network) on an
// individual file is silently skipped — the scan continues with any files that
// could be fetched.
//   - WorkflowsFetched=true if at least one file was successfully fetched.
//   - UsesPullRequestTarget=true if any successfully-fetched file contained
//     the trigger literal.
//   - "workflow_safety" is appended to partial only when ZERO files could be
//     fetched (total failure).
func processWorkflows(
	ctx context.Context,
	c *github.Client,
	owner, repo string,
	workflows []github.Workflow,
	partial *[]string,
) (hasCI, usesPRT, fetched bool) {
	for _, wf := range workflows {
		if wf.State == "active" {
			hasCI = true
		}
	}

	if len(workflows) == 0 {
		// Nothing to fetch; WorkflowsFetched stays false but no partial entry —
		// the repo simply has no workflows.
		return hasCI, false, false
	}

	// Attempt to fetch each workflow file. Tolerate per-file failures.
	for _, wf := range workflows {
		body, err := c.FileContent(ctx, owner, repo, wf.Path)
		if err != nil {
			// Skip this file; continue scanning others.
			continue
		}
		fetched = true
		if strings.Contains(string(body), "pull_request_target") {
			usesPRT = true
		}
	}

	if !fetched {
		// Could not fetch any file — safety state is unknown.
		*partial = append(*partial, "workflow_safety")
		return hasCI, false, false
	}
	return hasCI, usesPRT, true
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

// clampZero returns n if n >= 0, else 0.
func clampZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
