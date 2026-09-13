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
func Collect(ctx context.Context, c *github.Client, owner, repo string, now time.Time, opts ...Option) (score.RawMetrics, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	var raw score.RawMetrics

	o.emit(Progress{Stage: StageRepository, State: StageRunning})
	repoData, err := c.Repository(ctx, owner, repo)
	if err != nil {
		o.emit(Progress{Stage: StageRepository, State: StageFailed})
		return score.RawMetrics{}, err
	}
	applyRepo(&raw, repoData, now)
	header := raw
	o.emit(Progress{Stage: StageRepository, State: StageDone, Repo: &header})

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
		langs    languageResult
	)

	stage := func(name string, partial *string, fn func(context.Context) error) func() error {
		return func() error {
			o.emit(Progress{Stage: name, State: StageRunning})
			sctx := github.WithRetryObserver(gctx, func(_ string, attempt int) {
				o.emit(Progress{Stage: name, State: StageRetrying, Attempt: attempt})
			})
			err := fn(sctx)
			switch {
			case err != nil:
				o.emit(Progress{Stage: name, State: StageFailed})
			case *partial != "":
				o.emit(Progress{Stage: name, State: StageDegraded})
			default:
				o.emit(Progress{Stage: name, State: StageDone})
			}
			return err
		}
	}

	g.Go(stage(StageCommunity, &comm.partial, func(ctx context.Context) error {
		return collectCommunity(ctx, c, owner, repo, sem, &comm)
	}))
	g.Go(stage(StageContributors, &contrib.partial, func(ctx context.Context) error {
		return collectContributors(ctx, c, owner, repo, sem, &contrib)
	}))
	g.Go(stage(StageCommits, &commits.partial, func(ctx context.Context) error {
		return collectCommits(ctx, c, owner, repo, sem, now, &commits)
	}))
	g.Go(stage(StageReleases, &rels.partial, func(ctx context.Context) error {
		return collectReleases(ctx, c, owner, repo, sem, now, &rels)
	}))
	g.Go(stage(StageWorkflows, &flows.partial, func(ctx context.Context) error {
		return collectWorkflows(ctx, c, owner, repo, sem, &flows)
	}))
	g.Go(stage(StageClosedPulls, &closedPR.partial, func(ctx context.Context) error {
		return collectClosedPulls(ctx, c, owner, repo, sem, now, &closedPR)
	}))
	g.Go(stage(StageOpenPulls, &openPR.partial, func(ctx context.Context) error {
		return collectOpenPulls(ctx, c, owner, repo, sem, now, &openPR)
	}))
	g.Go(stage(StageIssueTTFR, &ttfr.partial, func(ctx context.Context) error {
		return collectTTFR(ctx, c, owner, repo, sem, now, &ttfr)
	}))
	g.Go(stage(StagePRCohort, &prCohort.partial, func(ctx context.Context) error {
		return collectPRCohort(ctx, c, owner, repo, sem, now, &prCohort)
	}))
	g.Go(stage(StageNewcomerLabels, &labels.partial, func(ctx context.Context) error {
		return collectNewcomerLabels(ctx, c, owner, repo, sem, &labels)
	}))
	g.Go(stage(StageLanguages, &langs.partial, func(ctx context.Context) error {
		return collectLanguages(ctx, c, owner, repo, sem, &langs)
	}))

	if err := g.Wait(); err != nil {
		return raw, err
	}

	assemble(&raw, &comm, &contrib, &commits, &rels, &flows, &closedPR, &openPR, &ttfr, &prCohort, &labels)
	raw.Languages = langs.shares
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
