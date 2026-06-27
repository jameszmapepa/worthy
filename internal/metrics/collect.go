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

	"github.com/charmbracelet/x/ansi"
	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/score"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// maxConcurrency bounds the number of GitHub HTTP calls Collect runs at once.
// ceiling: 8 keeps us far under GitHub's secondary concurrent-request limit
// (~100). The total request COUNT is unchanged by concurrency, so the primary
// rate limit (5000/hr auth, 60/hr anon) is unaffected; only the secondary
// concurrent limit applies. Raise cautiously if the secondary limit allows.
const maxConcurrency = 8

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
	applyRepo(&raw, repoData, now)

	// ------------------------------------------------------------------ //
	// All remaining calls are independent. Run them under a bounded       //
	// worker pool. Each collector writes ONLY its own result variable, so //
	// there is no shared mutable state and no mutex: race-freedom by       //
	// construction. Partial is assembled in a fixed canonical order AFTER  //
	// Wait, by this single goroutine, for deterministic output.            //
	//                                                                      //
	// gctx cancels the moment any collector returns a non-nil error.       //
	// Collectors return a non-nil error ONLY for context cancellation /    //
	// deadline; every other failure is recorded in their own partial slot  //
	// and returns nil (graceful degradation, unchanged from the serial     //
	// implementation).                                                     //
	// ------------------------------------------------------------------ //
	g, gctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(maxConcurrency)

	var (
		comm     communityResult
		contrib  contributorResult
		commits  commitResult
		rels     releaseResult
		flows    workflowResult
		closedPR closedPullsResult
		openPR   openPullsResult
		ttfr     ttfrResult
		prCohort prCohortResult
	)

	g.Go(func() error { return collectCommunity(gctx, c, owner, repo, sem, &comm) })
	g.Go(func() error { return collectContributors(gctx, c, owner, repo, sem, &contrib) })
	g.Go(func() error { return collectCommits(gctx, c, owner, repo, sem, &commits) })
	g.Go(func() error { return collectReleases(gctx, c, owner, repo, sem, now, &rels) })
	g.Go(func() error { return collectWorkflows(gctx, c, owner, repo, sem, &flows) })
	g.Go(func() error { return collectClosedPulls(gctx, c, owner, repo, sem, now, &closedPR) })
	g.Go(func() error { return collectOpenPulls(gctx, c, owner, repo, sem, now, &openPR) })
	g.Go(func() error { return collectTTFR(gctx, c, owner, repo, sem, now, &ttfr) })
	g.Go(func() error { return collectPRCohort(gctx, c, owner, repo, sem, now, &prCohort) })

	if err := g.Wait(); err != nil {
		return raw, err
	}

	assemble(&raw, &comm, &contrib, &commits, &rels, &flows, &closedPR, &openPR, &ttfr, &prCohort)
	return raw, nil
}

// applyRepo copies the core repository fields onto raw.
// A1: Description, Language, and LicenseSPDX come from the untrusted GitHub
// API and are later rendered raw into the terminal. Strip ANSI/OSC escape
// sequences at the ingestion boundary so control codes (screen clear, OSC-8
// hyperlinks, title changes) cannot reach the terminal regardless of how the
// TUI renders these strings.
func applyRepo(raw *score.RawMetrics, repoData *github.Repo, now time.Time) {
	raw.Stars = repoData.Stargazers
	raw.Watchers = repoData.Watchers
	raw.Forks = repoData.Forks
	raw.Fork = repoData.Fork // A4: plumb fork flag through to RawMetrics
	raw.Archived = repoData.Archived
	raw.Disabled = repoData.Disabled
	raw.Description = ansi.Strip(repoData.Description)
	raw.Language = ansi.Strip(repoData.Language)
	raw.DaysSinceLastPush = int(now.Sub(repoData.PushedAt).Hours() / 24)
	raw.RepoAgeDays = int(now.Sub(repoData.CreatedAt).Hours() / 24)
	if repoData.License != nil {
		raw.LicenseSPDX = ansi.Strip(repoData.License.SPDXID)
	}
}

// assemble merges the per-collector results onto raw and builds Partial in the
// canonical order. Running in the single Collect goroutine after Wait, it sees
// fully-published results with no synchronisation needed.
func assemble(
	raw *score.RawMetrics,
	comm *communityResult,
	contrib *contributorResult,
	commits *commitResult,
	rels *releaseResult,
	flows *workflowResult,
	closedPR *closedPullsResult,
	openPR *openPullsResult,
	ttfr *ttfrResult,
	prCohort *prCohortResult,
) {
	if comm.ok {
		raw.HealthPercentage = comm.healthPercentage
		raw.HasReadme = comm.hasReadme
		raw.HasContributing = comm.hasContributing
		raw.HasCodeOfConduct = comm.hasCodeOfConduct
		raw.HasSecurityPolicy = comm.hasSecurityPolicy
	}
	raw.TopContributorRecentShare = contrib.topShare
	raw.ContributorCount = contrib.count
	raw.CommitsLast52Weeks = commits.weekly
	raw.ReleaseCount = rels.count
	raw.DaysSinceLastRelease = rels.daysSince
	raw.HasSignedReleaseAssets = rels.signed
	raw.HasCI = flows.hasCI
	raw.UsesPullRequestTarget = flows.usesPRT
	raw.WorkflowsFetched = flows.fetched
	raw.MergedPRs = closedPR.merged
	raw.ClosedUnmergedPRs = closedPR.unmerged
	raw.NewcomerPRsMerged = closedPR.newcomerMerged
	raw.NewcomerPRsClosedUnmerged = closedPR.newcomerUnmerged
	// A2: open-PR ghosting metrics.
	raw.OpenPRCount = openPR.openCount
	raw.MedianOpenPRAgeDays = openPR.medianAgeDays
	raw.StaleNewcomerOpenPRs = openPR.staleNewcomerCount
	raw.MedianIssueFirstResponseHours = ttfr.median
	raw.RecentIssuesClosed = ttfr.recentIssuesClosed
	raw.RecentIssuesOpen = ttfr.recentIssuesOpen
	// A3: newcomer label counts derived from the issues slice.
	raw.GoodFirstIssues = ttfr.goodFirstIssues
	raw.HelpWantedIssues = ttfr.helpWantedIssues
	raw.RecentPRsMerged = prCohort.recentPRsMerged
	raw.RecentPRsOpen = prCohort.recentPRsOpen

	// Canonical Partial order; append only the degradations that occurred.
	appendPartial(raw, comm.partial)
	appendPartial(raw, contrib.partial)
	appendPartial(raw, commits.partial)
	appendPartial(raw, rels.partial)
	appendPartial(raw, flows.partial)
	appendPartial(raw, closedPR.partial)
	appendPartial(raw, openPR.partial)
	appendPartial(raw, ttfr.partial)
	appendPartial(raw, prCohort.partial)
}

// appendPartial appends a single non-empty partial marker to raw.Partial.
func appendPartial(raw *score.RawMetrics, marker string) {
	if marker != "" {
		raw.Partial = append(raw.Partial, marker)
	}
}

// isContextError reports whether err is a context cancellation or timeout.
// These abort the whole collection rather than degrading to Partial.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
