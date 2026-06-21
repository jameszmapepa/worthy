// Package github is a minimal, dependency-free client for the public GitHub
// REST API. It targets unauthenticated access (60 req/hr per IP) and treats a
// GITHUB_TOKEN as a pure opt-in that lifts the ceiling to 5,000 req/hr; the
// token is never required, prompted for, or persisted.
package github

import "time"

// User is the subset of a GitHub user/account we read.
type User struct {
	Login string `json:"login"`
	Type  string `json:"type"` // "User" or "Bot"
}

// License is the subset of a repository license we read.
type License struct {
	SPDXID string `json:"spdx_id"`
	Name   string `json:"name"`
}

// Repo is the subset of the repository object we read.
type Repo struct {
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	Stargazers    int       `json:"stargazers_count"`
	Watchers      int       `json:"subscribers_count"` // true watchers; watchers_count aliases stars
	Forks         int       `json:"forks_count"`
	OpenIssues    int       `json:"open_issues_count"` // includes open PRs
	License       *License  `json:"license"`
	PushedAt      time.Time `json:"pushed_at"`
	CreatedAt     time.Time `json:"created_at"`
	DefaultBranch string    `json:"default_branch"`
	Archived      bool      `json:"archived"`
	Disabled      bool      `json:"disabled"`
	Fork          bool      `json:"fork"`
}

// Issue is the subset of an issue we read. The GitHub /issues endpoint also
// returns pull requests; PullRequest is non-nil exactly when this item is a PR.
type Issue struct {
	Number      int        `json:"number"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Comments    int        `json:"comments"`
	User        User       `json:"user"`
	AuthorAssoc string     `json:"author_association"`
	PullRequest *struct{}  `json:"pull_request"`
}

// IsPullRequest reports whether this /issues item is actually a pull request.
func (i Issue) IsPullRequest() bool { return i.PullRequest != nil }

// Comment is the subset of an issue/PR comment we read.
type Comment struct {
	CreatedAt time.Time `json:"created_at"`
	User      User      `json:"user"`
}

// PullRequest is the subset of a pull request we read.
type PullRequest struct {
	Number      int        `json:"number"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	MergedAt    *time.Time `json:"merged_at"`
	User        User       `json:"user"`
	AuthorAssoc string     `json:"author_association"`
	MergedBy    *User      `json:"merged_by"`
}

// IsMerged reports whether the PR was merged (vs closed unmerged).
func (p PullRequest) IsMerged() bool { return p.MergedAt != nil }

// ContributorWeek is one week of a contributor's commit activity.
type ContributorWeek struct {
	Week      int64 `json:"w"` // unix timestamp of the start of the week
	Additions int   `json:"a"`
	Deletions int   `json:"d"`
	Commits   int   `json:"c"`
}

// ContributorStats is one contributor's all-time weekly commit series, as
// returned by GET /repos/{o}/{r}/stats/contributors.
type ContributorStats struct {
	Total  int               `json:"total"`
	Author User              `json:"author"`
	Weeks  []ContributorWeek `json:"weeks"`
}

// CommunityProfile is the subset of GET /repos/{o}/{r}/community/profile we
// read. HealthPercentage reflects only the PRESENCE of recommended files.
type CommunityProfile struct {
	HealthPercentage int `json:"health_percentage"`
	Files            struct {
		CodeOfConduct *struct{} `json:"code_of_conduct"`
		Contributing  *struct{} `json:"contributing"`
		License       *struct{} `json:"license"`
		Readme        *struct{} `json:"readme"`
		SecurityPol   *struct{} `json:"security"` //nolint:tagliatelle // GitHub field name
	} `json:"files"`
}

// Release is the subset of a release we read.
type Release struct {
	TagName     string     `json:"tag_name"`
	PublishedAt *time.Time `json:"published_at"`
	Prerelease  bool       `json:"prerelease"`
	Draft       bool       `json:"draft"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// Workflow is the subset of a GitHub Actions workflow we read.
type Workflow struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"` // "active", "disabled_manually", ...
}

// workflowList is the envelope for GET /repos/{o}/{r}/actions/workflows.
type workflowList struct {
	TotalCount int        `json:"total_count"`
	Workflows  []Workflow `json:"workflows"`
}

// CommitActivityWeek is one week of repository-wide commit activity from
// GET /repos/{o}/{r}/stats/commit_activity.
type CommitActivityWeek struct {
	Total int   `json:"total"`
	Week  int64 `json:"week"`
}
