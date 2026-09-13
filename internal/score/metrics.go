// Package score turns a RawMetrics snapshot into a scored Report.
package score

import "sort"

// RawMetrics is the pre-computed input to the scoring engine.
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
	WorkflowsFetched       bool

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
	Languages   []LanguageShare

	Partial []string
}

// LanguageShare is one language's share of the repository's code bytes.
type LanguageShare struct {
	Name    string
	Bytes   int
	Percent float64
}

// LanguageShares orders bytes-per-language by size and computes percentages.
func LanguageShares(bytesBy map[string]int) []LanguageShare {
	total := 0
	for _, b := range bytesBy {
		total += b
	}
	if total == 0 {
		return nil
	}
	out := make([]LanguageShare, 0, len(bytesBy))
	for name, b := range bytesBy {
		out = append(out, LanguageShare{Name: name, Bytes: b, Percent: float64(b) / float64(total) * 100})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Name < out[j].Name
	})
	return out
}
