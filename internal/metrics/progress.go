package metrics

import "github.com/jameszmapepa/worthy/internal/score"

// StageState is the lifecycle of one collection stage.
type StageState int

// Stage states, in lifecycle order.
const (
	StagePending StageState = iota
	StageRunning
	StageRetrying
	StageDone
	StageDegraded
	StageFailed
)

// Stage names, in the order Collect starts them.
const (
	StageRepository     = "repository"
	StageCommunity      = "community_profile"
	StageContributors   = "contributor_stats"
	StageCommits        = "commit_activity"
	StageReleases       = "releases"
	StageWorkflows      = "workflows"
	StageClosedPulls    = "closed_pulls"
	StageOpenPulls      = "open_pulls"
	StageIssueTTFR      = "issue_ttfr"
	StagePRCohort       = "pr_cohort"
	StageNewcomerLabels = "newcomer_labels"
)

// Stages lists every stage Collect reports, in start order.
var Stages = []string{
	StageRepository, StageCommunity, StageContributors, StageCommits, StageReleases,
	StageWorkflows, StageClosedPulls, StageOpenPulls, StageIssueTTFR, StagePRCohort,
	StageNewcomerLabels,
}

// Progress is one collection event.
type Progress struct {
	Stage   string
	State   StageState
	Attempt int
	Repo    *score.RawMetrics
}

// ProgressFunc receives Progress events.
type ProgressFunc func(Progress)

// Option configures Collect.
type Option func(*options)

type options struct {
	progress ProgressFunc
}

// WithProgress reports each stage transition to fn.
func WithProgress(fn ProgressFunc) Option {
	return func(o *options) { o.progress = fn }
}

func (o options) emit(p Progress) {
	if o.progress != nil {
		o.progress(p)
	}
}
