// Package metrics collects raw repository signals from the GitHub API into a score.RawMetrics value.
package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/score"
)

// ceiling: 8 stays far under GitHub's secondary concurrent-request limit; raise cautiously.
const maxConcurrency = 8

// Collect gathers repository health signals with now injected for deterministic testing; non-context errors degrade to RawMetrics.Partial.
func Collect(ctx context.Context, c *github.Client, owner, repo string, now time.Time) (score.RawMetrics, error) {
	var raw score.RawMetrics

	repoData, err := c.Repository(ctx, owner, repo)
	if err != nil {
		return score.RawMetrics{}, err
	}
	applyRepo(&raw, repoData, now)

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
		labels   newcomerLabelResult
	)

	g.Go(func() error { return collectCommunity(gctx, c, owner, repo, sem, &comm) })
	g.Go(func() error { return collectContributors(gctx, c, owner, repo, sem, &contrib) })
	g.Go(func() error { return collectCommits(gctx, c, owner, repo, sem, now, &commits) })
	g.Go(func() error { return collectReleases(gctx, c, owner, repo, sem, now, &rels) })
	g.Go(func() error { return collectWorkflows(gctx, c, owner, repo, sem, &flows) })
	g.Go(func() error { return collectClosedPulls(gctx, c, owner, repo, sem, now, &closedPR) })
	g.Go(func() error { return collectOpenPulls(gctx, c, owner, repo, sem, now, &openPR) })
	g.Go(func() error { return collectTTFR(gctx, c, owner, repo, sem, now, &ttfr) })
	g.Go(func() error { return collectPRCohort(gctx, c, owner, repo, sem, now, &prCohort) })
	g.Go(func() error { return collectNewcomerLabels(gctx, c, owner, repo, sem, &labels) })

	if err := g.Wait(); err != nil {
		return raw, err
	}

	assemble(&raw, &comm, &contrib, &commits, &rels, &flows, &closedPR, &openPR, &ttfr, &prCohort, &labels)
	return raw, nil
}

// applyRepo strips ANSI/OSC sequences from untrusted API fields before terminal rendering to prevent control-code injection.
func applyRepo(raw *score.RawMetrics, repoData *github.Repo, now time.Time) {
	raw.Stars = repoData.Stargazers
	raw.Watchers = repoData.Watchers
	raw.Forks = repoData.Forks
	raw.Fork = repoData.Fork
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
	labels *newcomerLabelResult,
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
	raw.CommitsPerWeekFallback = commits.weeklyAvg
	raw.HasCommitFallback = commits.hasFallback
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
	raw.OpenPRCount = openPR.openCount
	raw.MedianOpenPRAgeDays = openPR.medianAgeDays
	raw.StaleNewcomerOpenPRs = openPR.staleNewcomerCount
	raw.MedianIssueFirstResponseHours = ttfr.median
	raw.RecentIssuesClosed = ttfr.recentIssuesClosed
	raw.RecentIssuesOpen = ttfr.recentIssuesOpen
	raw.NewcomerLabeledOpen = labels.open
	raw.NewcomerLabeledAvailable = labels.available
	raw.NewcomerLabelsAvailable = labels.ok
	raw.RecentPRsMerged = prCohort.recentPRsMerged
	raw.RecentPRsOpen = prCohort.recentPRsOpen

	appendPartial(raw, comm.partial)
	appendPartial(raw, contrib.partial)
	appendPartial(raw, commits.partial)
	appendPartial(raw, rels.partial)
	appendPartial(raw, flows.partial)
	appendPartial(raw, closedPR.partial)
	appendPartial(raw, openPR.partial)
	appendPartial(raw, ttfr.partial)
	appendPartial(raw, prCohort.partial)
	appendPartial(raw, labels.partial)
}

func appendPartial(raw *score.RawMetrics, marker string) {
	if marker != "" {
		raw.Partial = append(raw.Partial, marker)
	}
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
