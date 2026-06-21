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
	CommitsLast52Weeks   []int // weekly repository-wide commit counts, oldest..newest
	DaysSinceLastPush    int   // recency of the last push to the default branch
	RepoAgeDays          int   // age of the repository (created_at proxy)
	OpenIssues           int   // open issues, excluding PRs
	ClosedIssues         int   // closed issues, excluding PRs
	OpenPRs              int   // open pull requests
	MergedPRs            int   // merged pull requests
	ClosedUnmergedPRs    int   // pull requests closed without merging
	ReleaseCount         int   // published releases (excludes draft/prerelease)
	DaysSinceLastRelease int   // recency of the most recent published release

	// Community / governance inputs.
	MedianIssueFirstResponseHours float64 // bot-filtered median time-to-first-response; <=0 means no data
	NewcomerPRsMerged             int     // newcomer PRs merged in the recent window
	NewcomerPRsClosedUnmerged     int     // newcomer PRs closed without merging
	TopContributorRecentShare     float64 // top login's fraction of last-12-week commits (0..1)
	ContributorCount              int     // contributors with >0 recent commits

	HasReadme         bool   // README present
	HasContributing   bool   // CONTRIBUTING present
	HasCodeOfConduct  bool   // CODE_OF_CONDUCT present
	HasLicense        bool   // LICENSE file present (community profile)
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

	// Vanity / sanity inputs.
	Stars    int // stargazers
	Forks    int // forks
	Watchers int // true watchers (subscribers), not the stars alias

	// Dead-repo flags.
	Archived bool // repository is archived (read-only)
	Disabled bool // repository is disabled

	// Partial records the names of metrics that were skipped during collection
	// due to rate-limit or 404 responses, so the UI can flag incomplete data.
	Partial []string
}
