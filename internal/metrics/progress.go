package metrics

import "github.com/jameszmapepa/worthy/internal/score"

// StageState is the lifecycle of one collection stage.
type StageState int

// Stage states, in lifecycle order; Degraded means the stage finished but
// its metric fell back to neutral.
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

// Progress is one collection event. Repo is set only on the repository
// stage's StageDone event and carries the header metadata (stars, description,
// license, age) so a UI can render it before the remaining stages finish.
type Progress struct {
	Stage   string
	State   StageState
	Attempt int
	Repo    *score.RawMetrics
}

// ProgressFunc receives Progress events. Stages run concurrently, so fn may be
// called from several goroutines at once.
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
