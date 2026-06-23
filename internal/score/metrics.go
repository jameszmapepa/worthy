// Package score turns a RawMetrics snapshot into a scored Report.
package score

// RawMetrics is the pre-computed input to the scoring engine; zero values are neutral defaults and Partial lists skipped metrics.
type RawMetrics struct {
	CommitsLast52Weeks []int

	CommitsPerWeekFallback float64
	HasCommitFallback      bool
	DaysSinceLastPush      int
	RepoAgeDays            int

	MergedPRs            int
	ClosedUnmergedPRs    int
	ReleaseCount         int
	DaysSinceLastRelease int

	RecentIssuesClosed int
	RecentIssuesOpen   int

	RecentPRsMerged int
	RecentPRsOpen   int

	MedianIssueFirstResponseHours float64
	NewcomerPRsMerged             int
	NewcomerPRsClosedUnmerged     int
	TopContributorRecentShare     float64
	ContributorCount              int

	HasReadme         bool
	HasContributing   bool
	HasCodeOfConduct  bool
	HasSecurityPolicy bool
	HealthPercentage  int
	LicenseSPDX       string

	HasCI                  bool
	HasSignedReleaseAssets bool
	UsesPullRequestTarget  bool
	// ceiling: single bool; replace with a richer status enum if per-file workflow fetching is added.
	WorkflowsFetched bool

	OpenPRCount          int
	MedianOpenPRAgeDays  float64
	StaleNewcomerOpenPRs int

	NewcomerLabeledOpen      int
	NewcomerLabeledAvailable int
	NewcomerLabelsAvailable  bool

	Stars    int
	Forks    int
	Watchers int
	Fork     bool

	Archived bool
	Disabled bool

	Description string
	Language    string

	Partial []string
}
