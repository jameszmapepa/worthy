package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

// CommunityProfile fetches community/profile metrics; returns *NotFoundError for forks because the endpoint 404s there.
func (c *Client) CommunityProfile(ctx context.Context, owner, repo string) (*CommunityProfile, error) {
	var p CommunityProfile
	path := fmt.Sprintf("/repos/%s/%s/community/profile",
		url.PathEscape(owner), url.PathEscape(repo))
	if err := c.get(ctx, path, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ContributorStats fetches per-contributor weekly commit series, retrying GitHub's 202 recompute responses.
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

// CommitCountSince counts commits since t using the per_page=1 Link rel="last" trick.
func (c *Client) CommitCountSince(ctx context.Context, owner, repo string, since time.Time) (int, error) {
	var sink []json.RawMessage
	path := fmt.Sprintf("/repos/%s/%s/commits?per_page=1&since=%s",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(since.UTC().Format(time.RFC3339)))
	header, err := c.getWithHeader(ctx, path, &sink)
	if err != nil {
		return 0, err
	}
	if last := lastPageFromLink(header); last > 0 {
		return last, nil
	}
	return len(sink), nil
}

// SearchIssueCount returns the total_count for a GitHub search query; the search endpoint has its own separate rate-limit budget.
func (c *Client) SearchIssueCount(ctx context.Context, query string) (int, error) {
	var r searchResult
	path := "/search/issues?per_page=1&q=" + url.QueryEscape(query)
	if err := c.get(ctx, path, &r); err != nil {
		return 0, err
	}
	return r.TotalCount, nil
}

func lastPageFromLink(h http.Header) int {
	for _, seg := range strings.Split(h.Get("Link"), ",") {
		if !strings.Contains(seg, `rel="last"`) {
			continue
		}
		for _, key := range []string{"?page=", "&page="} {
			i := strings.Index(seg, key)
			if i < 0 {
				continue
			}
			num := seg[i+len(key):]
			if end := strings.IndexAny(num, "&>"); end >= 0 {
				num = num[:end]
			}
			if n, err := strconv.Atoi(num); err == nil {
				return n
			}
		}
	}
	return 0
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

// RecentPulls fetches up to one page of pull requests in the given state, sorted by last update descending.
func (c *Client) RecentPulls(ctx context.Context, owner, repo, state string) ([]PullRequest, error) {
	var ps []PullRequest

	path := fmt.Sprintf("/repos/%s/%s/pulls?state=%s&per_page=%d&sort=updated&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(state), pageSize)
	if err := c.get(ctx, path, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// RecentPullsByCreation fetches up to one page of PRs sorted by creation date descending, unlike RecentPulls which sorts by update.
func (c *Client) RecentPullsByCreation(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	var ps []PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=%d&sort=created&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), pageSize)
	if err := c.get(ctx, path, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// RecentIssues fetches up to one page of issues; the GitHub API includes pull requests in this endpoint, so callers must filter using Issue.IsPullRequest.
func (c *Client) RecentIssues(ctx context.Context, owner, repo, state string) ([]Issue, error) {
	var is []Issue

	path := fmt.Sprintf("/repos/%s/%s/issues?state=%s&per_page=%d&sort=created&direction=desc",
		url.PathEscape(owner), url.PathEscape(repo), url.QueryEscape(state), pageSize)
	if err := c.get(ctx, path, &is); err != nil {
		return nil, err
	}
	return is, nil
}

// IssueComments fetches the comments on a single issue or PR, oldest first.
func (c *Client) IssueComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	var cs []Comment

	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), number, pageSize)
	if err := c.get(ctx, path, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

// FileContent fetches the raw bytes of a file, returning *NotFoundError if missing.
func (c *Client) FileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/contents/%s",
		url.PathEscape(owner), url.PathEscape(repo), encodeFilePath(path))
	return c.getRaw(ctx, endpoint)
}

// encodeFilePath percent-encodes each path segment while preserving slash separators.
// url.PathEscape would encode "/" as "%2F" and break directory structure.
func encodeFilePath(p string) string {
	segments := strings.Split(p, "/")
	escaped := make([]string, len(segments))
	for i, seg := range segments {
		escaped[i] = url.PathEscape(seg)
	}
	return strings.Join(escaped, "/")
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
