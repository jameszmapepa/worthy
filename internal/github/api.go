package github

import (
	"context"
	"fmt"
	"net/url"
)

// pageSize is GitHub's maximum items-per-page for list endpoints.
const pageSize = 100

// Repository fetches the core repository object.
func (c *Client) Repository(ctx context.Context, owner, repo string) (*Repo, error) {
	if owner == "" || repo == "" {
		return nil, errEmptyOwnerRepo
	}
	var r Repo
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CommunityProfile fetches the community/profile metrics. Returns a
// *NotFoundError for forks (the endpoint 404s there).
func (c *Client) CommunityProfile(ctx context.Context, owner, repo string) (*CommunityProfile, error) {
	var p CommunityProfile
	path := fmt.Sprintf("/repos/%s/%s/community/profile",
		url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ContributorStats fetches the all-time weekly commit series per contributor.
// Transparently waits out the 202-recompute response.
func (c *Client) ContributorStats(ctx context.Context, owner, repo string) ([]ContributorStats, error) {
	var s []ContributorStats
	path := fmt.Sprintf("/repos/%s/%s/stats/contributors",
		url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// CommitActivity fetches the last 52 weeks of repository-wide commit counts.
func (c *Client) CommitActivity(ctx context.Context, owner, repo string) ([]CommitActivityWeek, error) {
	var w []CommitActivityWeek
	path := fmt.Sprintf("/repos/%s/%s/stats/commit_activity",
		url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &w); err != nil {
		return nil, err
	}
	return w, nil
}

// Releases fetches up to limit recent releases (most recent first).
func (c *Client) Releases(ctx context.Context, owner, repo string, limit int) ([]Release, error) {
	var rs []Release
	path := fmt.Sprintf("/repos/%s/%s/releases?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), clampPage(limit))
	if err := c.get(ctx, path, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Workflows fetches the configured GitHub Actions workflows.
func (c *Client) Workflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	var wl workflowList
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows",
		url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &wl); err != nil {
		return nil, err
	}
	return wl.Workflows, nil
}

// RecentPulls fetches up to one page (100) of pull requests in the given state
// ("open", "closed", "all"), most-recently-updated first.
func (c *Client) RecentPulls(ctx context.Context, owner, repo, state string) ([]PullRequest, error) {
	var ps []PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d&sort=updated&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), state, pageSize)
	if err := c.get(ctx, path, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// RecentPullsByCreation fetches up to one page (100) of pull requests across
// all states, sorted by creation date descending. Used to build the 90-day PR
// creation cohort for pr_backlog without disturbing the existing RecentPulls
// call site (closed PRs, sorted by updated, for pr_acceptance/newcomer data).
func (c *Client) RecentPullsByCreation(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	var ps []PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=%d&sort=created&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), pageSize)
	if err := c.get(ctx, path, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// RecentIssues fetches up to one page (100) of issues (which, per the GitHub
// API, also includes pull requests — callers filter with Issue.IsPullRequest).
func (c *Client) RecentIssues(ctx context.Context, owner, repo, state string) ([]Issue, error) {
	var is []Issue
	path := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=%d&sort=created&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), state, pageSize)
	if err := c.get(ctx, path, &is); err != nil {
		return nil, err
	}
	return is, nil
}

// IssueComments fetches the comments on a single issue or PR, oldest first.
func (c *Client) IssueComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	var cs []Comment
	// number is an int — safe, no escaping needed.
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), number, pageSize)
	if err := c.get(ctx, path, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

// FileContent fetches the raw bytes of a single file in the repository using
// the raw-content Accept header. Returns a *NotFoundError for missing paths.
// The path argument is a slash-separated file path (e.g. ".github/workflows/ci.yml");
// each segment is percent-encoded so special characters cannot break out of the URL.
func (c *Client) FileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s",
		url.PathEscape(owner), url.PathEscape(repo), encodeFilePath(path))
	return c.getRaw(ctx, endpoint)
}

// encodeFilePath percent-encodes each path segment while preserving the
// slash separators so a path like ".github/workflows/ci.yml" becomes
// ".github%2Fworkflows%2Fci.yml" only for the individual segments — we keep
// slashes as literal "/" so the URL remains a valid path.
func encodeFilePath(p string) string {
	// url.PathEscape encodes "/" as "%2F" which would break the directory
	// structure. Split on "/" and escape each segment individually.
	segments := splitPath(p)
	escaped := make([]string, len(segments))
	for i, seg := range segments {
		escaped[i] = url.PathEscape(seg)
	}
	return joinPath(escaped)
}

// splitPath splits p on "/" without importing strings.
func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			parts = append(parts, p[start:i])
			start = i + 1
		}
	}
	parts = append(parts, p[start:])
	return parts
}

// joinPath joins segments with "/".
func joinPath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts) - 1
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, '/')
		}
		b = append(b, p...)
	}
	return string(b)
}

func clampPage(n int) int {
	if n < 1 {
		return 1
	}
	if n > pageSize {
		return pageSize
	}
	return n
}
