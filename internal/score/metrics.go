// Package score turns a RawMetrics snapshot of a public GitHub repository into
// a scored Report: per-indicator sub-scores, category aggregates, a weighted
// composite, conditional gates, and a letter grade.
//
// The package is PURE and deterministic: it imports nothing from the github or
// metrics packages, performs no I/O, and never calls time.Now. Every
// time-relative input arrives pre-computed on RawMetrics (for example
// DaysSinceLastPush), which makes the whole scoring model trivially unit
// testable against hand-built inputs.
package score

// RawMetrics is the input contract for scoring: a flat, pre-computed snapshot
// of one repository produced by metrics.Collect. All time-based fields are
// measured relative to an injected "now" at collection time, so scoring stays
// free of any clock.
//
// Fields default to their zero value when a metric could not be collected; the
// names of skipped metrics are recorded in Partial so the UI can flag partial
// data. Sub-score formulas treat zero as a documented neutral/no-data case
// where the spec calls for it (see score.go).
type RawMetrics struct {
	// Activity inputs.
	CommitsLast52Weeks []int // weekly repository-wide commit counts, oldest..newest
	DaysSinceLastPush  int   // recency of the last push to the default branch
	RepoAgeDays        int   // age of the repository (created_at proxy)
	// B6: Despite their names, MergedPRs and ClosedUnmergedPRs are NOT
	// all-time totals. The collector fetches the most recently-updated 100
	// closed pull requests (one API page, state=closed, sort=updated-desc,
	// per_page=100). "No closed PRs" therefore means "no recently-updated
	// closed PRs in that page", not "no PRs ever closed". See
	// metrics.Collect → collectClosedPulls.
	MergedPRs            int // merged PRs in the most recently-updated 100 closed PRs
	ClosedUnmergedPRs    int // closed-without-merging PRs in the same page
	ReleaseCount         int // published releases (excludes draft/prerelease)
	DaysSinceLastRelease int // recency of the most recent published release

	// Recent 90-day creation cohort counts (zero = neutral no-data via ratioScore).
	// Source: non-PR issues with CreatedAt >= now-90d, derived from the existing
	// RecentIssues fetch (no additional API call).
	RecentIssuesClosed int
	RecentIssuesOpen   int
	// Source: PRs with CreatedAt >= now-90d, from a new RecentPullsByCreation call
	// (state=all, sort=created desc). Closed-unmerged PRs are excluded from both
	// numerator and denominator.
	RecentPRsMerged int
	RecentPRsOpen   int

	// Community / governance inputs.
	MedianIssueFirstResponseHours float64 // bot-filtered median time-to-first-response; <=0 means no data
	NewcomerPRsMerged             int     // newcomer PRs merged in the recent window
	NewcomerPRsClosedUnmerged     int     // newcomer PRs closed without merging
	TopContributorRecentShare     float64 // top login's fraction of last-12-week commits (0..1)
	ContributorCount              int     // contributors with >0 recent commits

	HasReadme         bool   // README present
	HasContributing   bool   // CONTRIBUTING present
	HasCodeOfConduct  bool   // CODE_OF_CONDUCT present
	HasSecurityPolicy bool   // SECURITY policy present
	HealthPercentage  int    // GitHub community-profile presence-only score (0..100)
	LicenseSPDX       string // SPDX id; "" or "NOASSERTION" means no recognized license

	// Security / integrity inputs.
	HasCI                  bool // at least one active GitHub Actions workflow
	HasSignedReleaseAssets bool // any release asset matching .asc/.sig/.sigstore/.intoto.jsonl
	UsesPullRequestTarget  bool // any workflow file uses pull_request_target
	// WorkflowsFetched reports whether workflow file contents were actually
	// retrieved. The RawMetrics table notes UsesPullRequestTarget is a
	// best-effort signal that is false when files were not fetched; the
	// workflow_safety sub-score needs to distinguish "fetched and safe" (100)
	// from "not fetched / unknown" (70), so collection records that here.
	// ceiling: a single bool; if we later fetch workflows per-file, replace
	// with a richer status enum.
	WorkflowsFetched bool

	// Open PR ghosting (A2): signals whether maintainers leave newcomer PRs
	// open indefinitely. Collected from the first 100 open PRs (page cap);
	// zero values are the neutral no-data case.
	OpenPRCount          int     // total open PRs at collection time (up to 100)
	MedianOpenPRAgeDays  float64 // median age in days of open PRs; 0 = no open PRs
	StaleNewcomerOpenPRs int     // open newcomer PRs opened >30 days ago

	// Newcomer-friendliness (A3): derived from already-fetched issue data at
	// zero extra API cost; counts are capped at 100.
	GoodFirstIssues  int // open non-PR issues labelled "good first issue" or "good-first-issue"
	HelpWantedIssues int // open non-PR issues labelled "help wanted" or "help-wanted"

	// Vanity / sanity inputs.
	Stars    int  // stargazers
	Forks    int  // forks
	Watchers int  // true watchers (subscribers), not the stars alias
	Fork     bool // repository is a fork (A4)

	// Dead-repo flags.
	Archived bool // repository is archived (read-only)
	Disabled bool // repository is disabled

	// Presentation metadata (not scored; surfaced in the TUI header).
	Description string // repository description ("" if none)
	Language    string // primary language ("" if none)

	// Partial records the names of metrics that were skipped during collection
	// due to rate-limit or 404 responses, so the UI can flag incomplete data.
	Partial []string
}
