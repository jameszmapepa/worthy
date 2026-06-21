package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// ---------------------------------------------------------------------------
// Test server infrastructure
// ---------------------------------------------------------------------------

// fixture is a response payload — either a pre-serialised JSON string or a
// struct that will be marshalled on first use.
type fixture struct {
	status int
	body   string // raw JSON; if empty, status only
}

// mux builds an *httptest.Server whose handler dispatches on exact URL path
// (query params ignored for routing). Each path maps to one fixture.
// Unregistered paths return 500 so tests fail loudly if Collect calls an
// unexpected endpoint.
func mux(routes map[string]fixture) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		f, ok := routes[path]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":"unexpected path %s"}`, path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		if f.body != "" {
			_, _ = fmt.Fprint(w, f.body)
		}
	}))
}

// client wraps github.NewClient pointed at srv with fast retries + no token.
func client(srv *httptest.Server) *github.Client {
	return github.NewClient(
		github.WithBaseURL(srv.URL),
		github.WithHTTPClient(srv.Client()),
		github.WithRetry(2, time.Millisecond),
		github.WithToken(""), // prevent picking up GITHUB_TOKEN from env
	)
}

// ---------------------------------------------------------------------------
// Shared fixture JSON strings
// ---------------------------------------------------------------------------

const repoJSON = `{
	"full_name":"acme/widget",
	"stargazers_count":1500,
	"subscribers_count":80,
	"forks_count":200,
	"open_issues_count":25,
	"archived":false,
	"disabled":false,
	"fork":false,
	"pushed_at":"2026-05-22T10:00:00Z",
	"created_at":"2020-01-01T00:00:00Z",
	"default_branch":"main",
	"license":{"spdx_id":"MIT","name":"MIT License"}
}`

// communityJSON has readme + license + security present; contributing + CoC absent.
const communityJSON = `{
	"health_percentage":72,
	"files":{
		"readme":{"url":"x"},
		"contributing":null,
		"license":{"url":"x"},
		"code_of_conduct":null,
		"security":{"url":"x"}
	}
}`

// contributorStatsJSON: two contributors, alice dominates recent weeks.
// We populate weeks[0..51]; last 12 are weeks[40..51].
// alice: 10 commits/wk in last 12, bob: 2 commits/wk in last 12.
func buildContributorStatsJSON() string {
	type week struct {
		W int64 `json:"w"`
		A int   `json:"a"`
		D int   `json:"d"`
		C int   `json:"c"`
	}
	type contrib struct {
		Total  int `json:"total"`
		Author struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"author"`
		Weeks []week `json:"weeks"`
	}

	baseWeek := int64(1600000000)
	makeWeeks := func(recent int) []week {
		ws := make([]week, 52)
		for i := range ws {
			ws[i] = week{W: baseWeek + int64(i)*604800, A: 1, D: 0, C: 1}
		}
		// last 12 weeks get the specified count
		for i := 40; i < 52; i++ {
			ws[i].C = recent
		}
		return ws
	}

	alice := contrib{Total: 52 * 11}
	alice.Author.Login = "alice"
	alice.Author.Type = "User"
	alice.Weeks = makeWeeks(10)

	bob := contrib{Total: 52 * 3}
	bob.Author.Login = "bob"
	bob.Author.Type = "User"
	bob.Weeks = makeWeeks(2)

	b, _ := json.Marshal([]contrib{alice, bob})
	return string(b)
}

// commitActivityJSON: 52 weeks with varying totals. Last 12 sum to ~60.
func buildCommitActivityJSON() string {
	type week struct {
		Total int   `json:"total"`
		Week  int64 `json:"week"`
	}
	ws := make([]week, 52)
	base := int64(1600000000)
	for i := range ws {
		ws[i] = week{Total: i % 5, Week: base + int64(i)*604800}
	}
	b, _ := json.Marshal(ws)
	return string(b)
}

// releasesJSON: two real releases + one draft + one pre-release.
// Second real release has a .asc signature asset.
func buildReleasesJSON(now time.Time) string {
	t1 := now.Add(-45 * 24 * time.Hour).UTC().Format(time.RFC3339)
	t2 := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)
	return fmt.Sprintf(`[
		{"tag_name":"v2.0.0","prerelease":false,"draft":false,
		 "published_at":%q,
		 "assets":[{"name":"widget.tar.gz"},{"name":"widget.tar.gz.asc"}]},
		{"tag_name":"v1.9.0","prerelease":false,"draft":false,
		 "published_at":%q,
		 "assets":[{"name":"widget.tar.gz"}]},
		{"tag_name":"v1.9.1-rc1","prerelease":true,"draft":false,
		 "published_at":%q,"assets":[]},
		{"tag_name":"v1.9.0-draft","prerelease":false,"draft":true,
		 "published_at":%q,"assets":[]}
	]`, t1, t2, t1, t2)
}

const workflowsJSON = `{
	"total_count":2,
	"workflows":[
		{"name":"CI","path":".github/workflows/ci.yml","state":"active"},
		{"name":"Release","path":".github/workflows/release.yml","state":"disabled_manually"}
	]
}`

// ciYAML does NOT use pull_request_target.
const ciYAML = `
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`

// releaseYAML uses pull_request_target.
const releaseYAML = `
on:
  pull_request_target:
    branches: [main]
jobs:
  release:
    runs-on: ubuntu-latest
`

// closedPRsJSON: 4 PRs — 2 merged, 1 closed-unmerged, 1 merged by self (excluded from newcomer).
func buildClosedPRsJSON(now time.Time) string {
	t := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	tOld := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339) // outside 90-day window
	return fmt.Sprintf(`[
		{"number":1,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"alice","type":"User"},"author_association":"FIRST_TIME_CONTRIBUTOR",
		 "merged_by":{"login":"maintainer","type":"User"}},
		{"number":2,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":null,
		 "user":{"login":"bob","type":"User"},"author_association":"NONE",
		 "merged_by":null},
		{"number":3,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"carol","type":"User"},"author_association":"CONTRIBUTOR",
		 "merged_by":{"login":"carol","type":"User"}},
		{"number":4,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"dave","type":"User"},"author_association":"MEMBER",
		 "merged_by":{"login":"maintainer","type":"User"}}
	]`,
		t, t, t, // PR1: newcomer merged by maintainer (counts)
		t, t, // PR2: newcomer closed-unmerged (counts)
		t, t, t, // PR3: newcomer merged by SELF (excluded from newcomer stats)
		tOld, tOld, tOld, // PR4: MEMBER (not newcomer), and outside window
	)
}

// openPRsLinkJSON is a per_page=1 response with Link: rel="last" page=12.
const openPRsLinkJSON = `[{"number":99,"state":"open"}]`

// closedPRsLinkJSON is a per_page=1 response with Link: rel="last" page=25.
const closedPRsLinkJSON = `[{"number":98,"state":"closed"}]`

// openIssuesLinkJSON: last page=40.
const openIssuesLinkJSON = `[{"number":50,"state":"open"}]`

// closedIssuesLinkJSON: last page=80.
const closedIssuesLinkJSON = `[{"number":51,"state":"closed"}]`

// recentIssuesJSON: 3 issues — 1 is actually a PR (has pull_request field),
// 2 are real issues. This exercises IsPullRequest filtering.
func buildRecentIssuesJSON(now time.Time) string {
	t := now.Add(-10 * time.Hour).UTC().Format(time.RFC3339)
	return fmt.Sprintf(`[
		{"number":10,"state":"open","created_at":%q,"closed_at":null,
		 "comments":2,"user":{"login":"dave","type":"User"},
		 "author_association":"NONE","pull_request":{}},
		{"number":11,"state":"open","created_at":%q,"closed_at":null,
		 "comments":1,"user":{"login":"eve","type":"User"},
		 "author_association":"NONE","pull_request":null},
		{"number":12,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"frank","type":"User"},
		 "author_association":"NONE","pull_request":null}
	]`, t, t, t)
}

// buildIssueCommentsJSON builds comments for a given issue. firstBot=true
// means the first comment is a bot.
func buildIssueCommentsJSON(now time.Time, issueCreated time.Time, firstBot bool) string {
	t1 := issueCreated.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	t2 := issueCreated.Add(5 * time.Hour).UTC().Format(time.RFC3339)
	_ = now // kept for future use

	if firstBot {
		return fmt.Sprintf(`[
			{"created_at":%q,"user":{"login":"stale[bot]","type":"Bot"}},
			{"created_at":%q,"user":{"login":"maintainer","type":"User"}}
		]`, t1, t2)
	}
	return fmt.Sprintf(`[
		{"created_at":%q,"user":{"login":"maintainer","type":"User"}}
	]`, t1)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// rateLimitFixture returns a fixture that mimics a GitHub rate-limit response.
func rateLimitFixture() fixture {
	reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
	return fixture{
		status: http.StatusForbidden,
		body: fmt.Sprintf(`{"message":"API rate limit exceeded","documentation_url":"x"}`) + "\n" +
			fmt.Sprintf(`x-ratelimit-remaining: 0; reset: %s`, reset),
	}
}

// rateLimitHandler returns an http.HandlerFunc that writes rate-limit headers.
func rateLimitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Reset", reset)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}
}

// notFoundHandler returns a 404 handler.
func notFoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}
}

// containsAll checks s contains all substrings in subs.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Full happy-path test
// ---------------------------------------------------------------------------

func TestCollect_HappyPath(t *testing.T) {
	// Fixed "now" so all time computations are deterministic.
	// pushed_at = 2026-05-22 → 31 days before now (2026-06-22).
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	pushedAt, _ := time.Parse(time.RFC3339, "2026-05-22T10:00:00Z")
	expectedDaysSincePush := int(now.Sub(pushedAt).Hours() / 24) // ~30

	createdAt, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	expectedRepoAge := int(now.Sub(createdAt).Hours() / 24) // ~2364

	releasesBody := buildReleasesJSON(now)
	closedPRsBody := buildClosedPRsJSON(now)
	recentIssuesBody := buildRecentIssuesJSON(now)

	// Issue 11 (eve, created 10h before now) → first comment 2h later = 2h TTFR.
	// Issue 12 (frank, no comments) → no qualifying response.
	issueCreated := now.Add(-10 * time.Hour)
	issue11Comments := buildIssueCommentsJSON(now, issueCreated, false) // 2h response, human
	issue12Comments := `[]`                                             // no comments

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Core repo
		case path == "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)

		// Community profile
		case path == "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)

		// Contributor stats
		case path == "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildContributorStatsJSON())

		// Commit activity
		case path == "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildCommitActivityJSON())

		// Releases
		case path == "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releasesBody)

		// Workflows list
		case path == "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON)

		// Workflow file contents (raw)
		case path == "/repos/acme/widget/contents/.github/workflows/ci.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, ciYAML)
		case path == "/repos/acme/widget/contents/.github/workflows/release.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releaseYAML)

		// RecentPulls — closed page for newcomer analysis (per_page=100 checked BEFORE per_page=1 to avoid substring match)
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "closed" && r.URL.Query().Get("per_page") == "100":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsBody)

		// CountByState — issues open (per_page=1 with Link header)
		case path == "/repos/acme/widget/issues" && r.URL.Query().Get("state") == "open" && r.URL.Query().Get("per_page") == "1":
			w.Header().Set("Link", `<https://api.github.com/repos/acme/widget/issues?page=40>; rel="last"`)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, openIssuesLinkJSON)

		// CountByState — issues closed (per_page=1 with Link header)
		case path == "/repos/acme/widget/issues" && r.URL.Query().Get("state") == "closed" && r.URL.Query().Get("per_page") == "1":
			w.Header().Set("Link", `<https://api.github.com/repos/acme/widget/issues?page=80>; rel="last"`)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedIssuesLinkJSON)

		// CountByState — pulls open
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "open" && r.URL.Query().Get("per_page") == "1":
			w.Header().Set("Link", `<https://api.github.com/repos/acme/widget/pulls?page=12>; rel="last"`)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, openPRsLinkJSON)

		// CountByState — pulls closed
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "closed" && r.URL.Query().Get("per_page") == "1":
			w.Header().Set("Link", `<https://api.github.com/repos/acme/widget/pulls?page=25>; rel="last"`)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsLinkJSON)

		// RecentIssues — all, for TTFR sampling
		case path == "/repos/acme/widget/issues" && r.URL.Query().Get("state") == "all" && r.URL.Query().Get("per_page") == "100":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, recentIssuesBody)

		// IssueComments for issue 11 and 12 (10 is a PR, filtered out)
		case path == "/repos/acme/widget/issues/11/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue11Comments)
		case path == "/repos/acme/widget/issues/12/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue12Comments)

		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":"unexpected path %s query %s"}`, path, r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// --- Repository core ---
	if got.Stars != 1500 {
		t.Errorf("Stars = %d; want 1500", got.Stars)
	}
	if got.Forks != 200 {
		t.Errorf("Forks = %d; want 200", got.Forks)
	}
	if got.Watchers != 80 {
		t.Errorf("Watchers = %d; want 80", got.Watchers)
	}
	if got.Archived {
		t.Error("Archived should be false")
	}
	if got.Disabled {
		t.Error("Disabled should be false")
	}
	if got.LicenseSPDX != "MIT" {
		t.Errorf("LicenseSPDX = %q; want %q", got.LicenseSPDX, "MIT")
	}
	if got.DaysSinceLastPush != expectedDaysSincePush {
		t.Errorf("DaysSinceLastPush = %d; want ~%d", got.DaysSinceLastPush, expectedDaysSincePush)
	}
	if got.RepoAgeDays != expectedRepoAge {
		t.Errorf("RepoAgeDays = %d; want ~%d", got.RepoAgeDays, expectedRepoAge)
	}

	// --- Governance ---
	if got.HealthPercentage != 72 {
		t.Errorf("HealthPercentage = %d; want 72", got.HealthPercentage)
	}
	if !got.HasReadme {
		t.Error("HasReadme should be true")
	}
	if got.HasContributing {
		t.Error("HasContributing should be false")
	}
	if !got.HasLicense {
		t.Error("HasLicense should be true")
	}
	if got.HasCodeOfConduct {
		t.Error("HasCodeOfConduct should be false")
	}
	if !got.HasSecurityPolicy {
		t.Error("HasSecurityPolicy should be true")
	}

	// --- Contributor stats ---
	// alice: 10*12=120, bob: 2*12=24 → total=144, alice share=120/144≈0.833
	wantAliceShare := 120.0 / 144.0
	if got.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d; want 2", got.ContributorCount)
	}
	if got.TopContributorRecentShare < wantAliceShare-0.01 || got.TopContributorRecentShare > wantAliceShare+0.01 {
		t.Errorf("TopContributorRecentShare = %.4f; want ≈%.4f", got.TopContributorRecentShare, wantAliceShare)
	}

	// --- CommitsLast52Weeks ---
	if len(got.CommitsLast52Weeks) != 52 {
		t.Errorf("len(CommitsLast52Weeks) = %d; want 52", len(got.CommitsLast52Weeks))
	}

	// --- Releases ---
	// Only 2 non-draft, non-pre-release; most recent is 45 days ago.
	if got.ReleaseCount != 2 {
		t.Errorf("ReleaseCount = %d; want 2", got.ReleaseCount)
	}
	if got.DaysSinceLastRelease != 45 {
		t.Errorf("DaysSinceLastRelease = %d; want 45", got.DaysSinceLastRelease)
	}
	if !got.HasSignedReleaseAssets {
		t.Error("HasSignedReleaseAssets should be true (.asc asset present)")
	}

	// --- CI ---
	if !got.HasCI {
		t.Error("HasCI should be true (one active workflow)")
	}

	// --- Workflow safety ---
	if !got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be true")
	}
	if !got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be true (release.yml uses it)")
	}

	// --- Issue counts (excluding PRs) ---
	// open issues link=40, open PRs=12 → OpenIssues = 40-12 = 28
	// closed issues link=80, closed PRs=25 → ClosedIssues = 80-25 = 55
	if got.OpenIssues != 28 {
		t.Errorf("OpenIssues = %d; want 28 (40 issues - 12 PRs)", got.OpenIssues)
	}
	if got.ClosedIssues != 55 {
		t.Errorf("ClosedIssues = %d; want 55 (80 issues - 25 PRs)", got.ClosedIssues)
	}

	// --- PR outcome split ---
	if got.OpenPRs != 12 {
		t.Errorf("OpenPRs = %d; want 12", got.OpenPRs)
	}
	// Closed PRs page: PR1 merged, PR2 closed-unmerged, PR3 merged, PR4 merged.
	// MergedPRs = PRs where IsMerged()==true from sample = 3; ClosedUnmergedPRs = 1.
	if got.MergedPRs != 3 {
		t.Errorf("MergedPRs = %d; want 3", got.MergedPRs)
	}
	if got.ClosedUnmergedPRs != 1 {
		t.Errorf("ClosedUnmergedPRs = %d; want 1", got.ClosedUnmergedPRs)
	}

	// --- Newcomer PRs ---
	// PR1: FIRST_TIME_CONTRIBUTOR, merged by maintainer (not self), within 90d → NewcomerPRsMerged
	// PR2: NONE, closed-unmerged, within 90d → NewcomerPRsClosedUnmerged
	// PR3: CONTRIBUTOR, but self-merged → excluded from newcomer counts
	// PR4: MEMBER → not newcomer
	if got.NewcomerPRsMerged != 1 {
		t.Errorf("NewcomerPRsMerged = %d; want 1", got.NewcomerPRsMerged)
	}
	if got.NewcomerPRsClosedUnmerged != 1 {
		t.Errorf("NewcomerPRsClosedUnmerged = %d; want 1", got.NewcomerPRsClosedUnmerged)
	}

	// --- TTFR ---
	// Issue 10 is a PR → filtered. Issue 11: eve, first response by maintainer 2h later.
	// Issue 12: frank, no comments → no response. Median of [2.0] = 2.0h.
	if got.MedianIssueFirstResponseHours < 1.9 || got.MedianIssueFirstResponseHours > 2.1 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≈2.0", got.MedianIssueFirstResponseHours)
	}

	// No partial metrics in the happy path.
	if len(got.Partial) != 0 {
		t.Errorf("Partial = %v; want empty", got.Partial)
	}
}

// ---------------------------------------------------------------------------
// First-call (Repository) failure → return error
// ---------------------------------------------------------------------------

func TestCollect_RepositoryNotFound_ReturnsError(t *testing.T) {
	now := time.Now()
	srv := httptest.NewServer(notFoundHandler())
	defer srv.Close()

	c := client(srv)
	_, err := Collect(context.Background(), c, "no", "such", now)
	if err == nil {
		t.Fatal("expected error when Repository returns 404")
	}
	var nfe *github.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("want *github.NotFoundError; got %T: %v", err, err)
	}
}

func TestCollect_RepositoryRateLimited_ReturnsError(t *testing.T) {
	now := time.Now()
	srv := httptest.NewServer(rateLimitHandler())
	defer srv.Close()

	c := client(srv)
	_, err := Collect(context.Background(), c, "acme", "widget", now)
	if err == nil {
		t.Fatal("expected error when Repository returns rate limit")
	}
	var rle *github.RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("want *github.RateLimitError; got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Graceful degradation — 404 on a non-fatal endpoint
// ---------------------------------------------------------------------------

func TestCollect_CommunityProfile404_RecordedInPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			notFoundHandler()(w, r)

		// Stubs for the remaining endpoints so Collect can finish.
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should not fail on community/profile 404; got: %v", err)
	}
	if !containsStr(got.Partial, "community_profile") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "community_profile")
	}
	// Governance fields should remain zero.
	if got.HasReadme || got.HasLicense || got.HasSecurityPolicy {
		t.Error("governance fields should be false when community/profile is missing")
	}
}

// ---------------------------------------------------------------------------
// Graceful degradation — rate limit on contributor stats
// ---------------------------------------------------------------------------

func TestCollect_ContributorStatsRateLimited_RecordedInPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			rateLimitHandler()(w, r)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should not fail when contributor stats is rate-limited; got: %v", err)
	}
	if !containsStr(got.Partial, "contributor_stats") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "contributor_stats")
	}
	// TopContributorRecentShare and ContributorCount stay zero.
	if got.TopContributorRecentShare != 0 {
		t.Errorf("TopContributorRecentShare = %.2f; want 0", got.TopContributorRecentShare)
	}
}

// ---------------------------------------------------------------------------
// Bot-filtering in TTFR: first comment by a bot should be skipped
// ---------------------------------------------------------------------------

func TestCollect_TTFRBotFiltered(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	issueCreated := now.Add(-20 * time.Hour)
	// First comment is a bot (2h after creation), second is human (5h after).
	// Bot-filtered TTFR should be 5h, not 2h.
	commentsJSON := buildIssueCommentsJSON(now, issueCreated, true /* firstBot */)

	oneIssueJSON := fmt.Sprintf(`[
		{"number":20,"state":"open","created_at":%q,"closed_at":null,
		 "comments":2,"user":{"login":"reporter","type":"User"},
		 "author_association":"NONE","pull_request":null}
	]`, issueCreated.UTC().Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			if strings.Contains(r.URL.RawQuery, "state=all") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oneIssueJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		case "/repos/acme/widget/issues/20/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, commentsJSON)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// First human response is 5h after creation.
	if got.MedianIssueFirstResponseHours < 4.9 || got.MedianIssueFirstResponseHours > 5.1 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≈5.0 (bot filtered)", got.MedianIssueFirstResponseHours)
	}
}

// ---------------------------------------------------------------------------
// TTFR: first response must be by a different login than issue author
// ---------------------------------------------------------------------------

func TestCollect_TTFRSameAuthorFiltered(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	issueCreated := now.Add(-10 * time.Hour)

	// Both comments are by the issue author themselves — no valid first response.
	selfComments := fmt.Sprintf(`[
		{"created_at":%q,"user":{"login":"reporter","type":"User"}},
		{"created_at":%q,"user":{"login":"reporter","type":"User"}}
	]`,
		issueCreated.Add(2*time.Hour).UTC().Format(time.RFC3339),
		issueCreated.Add(4*time.Hour).UTC().Format(time.RFC3339),
	)

	oneIssueJSON := fmt.Sprintf(`[
		{"number":30,"state":"open","created_at":%q,"closed_at":null,
		 "comments":2,"user":{"login":"reporter","type":"User"},
		 "author_association":"NONE","pull_request":null}
	]`, issueCreated.UTC().Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			if strings.Contains(r.URL.RawQuery, "state=all") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oneIssueJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		case "/repos/acme/widget/issues/30/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, selfComments)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// No valid first response → MedianIssueFirstResponseHours ≤ 0
	if got.MedianIssueFirstResponseHours > 0 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≤0 (all comments by author)", got.MedianIssueFirstResponseHours)
	}
}

// ---------------------------------------------------------------------------
// Self-merge exclusion in newcomer PRs
// ---------------------------------------------------------------------------

func TestCollect_NewcomerSelfMergeExcluded(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	t1 := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// Single PR: newcomer FIRST_TIME_CONTRIBUTOR, merged by themselves.
	// Should produce 0 NewcomerPRsMerged.
	selfMergeJSON := fmt.Sprintf(`[
		{"number":5,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"newcomer","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR",
		 "merged_by":{"login":"newcomer","type":"User"}}
	]`, t1, t1, t1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			if strings.Contains(r.URL.RawQuery, "state=closed") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, selfMergeJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.NewcomerPRsMerged != 0 {
		t.Errorf("NewcomerPRsMerged = %d; want 0 (self-merge excluded)", got.NewcomerPRsMerged)
	}
}

// ---------------------------------------------------------------------------
// Newcomer 90-day window: PR outside window excluded
// ---------------------------------------------------------------------------

func TestCollect_NewcomerOutsideWindow_Excluded(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	// PR closed 200 days ago — outside 90d window.
	tOld := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)

	oldPRJSON := fmt.Sprintf(`[
		{"number":6,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"alice","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR",
		 "merged_by":{"login":"maintainer","type":"User"}}
	]`, tOld, tOld, tOld)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			if strings.Contains(r.URL.RawQuery, "state=closed") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oldPRJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.NewcomerPRsMerged != 0 {
		t.Errorf("NewcomerPRsMerged = %d; want 0 (PR outside 90d window)", got.NewcomerPRsMerged)
	}
}

// ---------------------------------------------------------------------------
// Issue count minus PR math: clamp to ≥0
// ---------------------------------------------------------------------------

func TestCollect_IssueCountMinusPRClamp(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// open issues=5, open PRs=10 → OpenIssues should be clamped to 0 (not -5)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			q := r.URL.Query()
			if q.Get("state") == "open" && q.Get("per_page") == "1" {
				// 5 open issues — Link header must be set BEFORE WriteHeader.
				w.Header().Set("Link", `<https://x?page=5>; rel="last"`)
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[{}]`)
			} else if q.Get("state") == "closed" && q.Get("per_page") == "1" {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[{}]`)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		case "/repos/acme/widget/pulls":
			q := r.URL.Query()
			if q.Get("state") == "open" && q.Get("per_page") == "1" {
				// 10 open PRs — more than the 5 open issues.
				w.Header().Set("Link", `<https://x?page=10>; rel="last"`)
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[{}]`)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.OpenIssues < 0 {
		t.Errorf("OpenIssues = %d; want ≥0 (clamped)", got.OpenIssues)
	}
}

// ---------------------------------------------------------------------------
// WorkflowsFetched=false when workflow file fetch fails
// ---------------------------------------------------------------------------

func TestCollect_WorkflowFileFetchFails_WorkflowsFetchedFalse(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON) // returns 2 workflows
		// All workflow file fetches return rate-limit
		case "/repos/acme/widget/contents/.github/workflows/ci.yml",
			"/repos/acme/widget/contents/.github/workflows/release.yml":
			rateLimitHandler()(w, r)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be false when file fetches fail")
	}
	if got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be false when files not fetched")
	}
	if !containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "workflow_safety")
	}
}

// ---------------------------------------------------------------------------
// No active workflows → HasCI=false
// ---------------------------------------------------------------------------

func TestCollect_NoActiveWorkflows_HasCIFalse(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	const noActiveWorkflowsJSON = `{
		"total_count":1,
		"workflows":[
			{"name":"Disabled","path":".github/workflows/disabled.yml","state":"disabled_manually"}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, noActiveWorkflowsJSON)
		case "/repos/acme/widget/contents/.github/workflows/disabled.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, ciYAML)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.HasCI {
		t.Error("HasCI should be false when no workflows are active")
	}
}

// ---------------------------------------------------------------------------
// Releases: exclude draft + prerelease; signed asset detection
// ---------------------------------------------------------------------------

func TestCollect_ReleasesFilteredAndSigned(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	t1 := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// 1 real release with .sigstore asset; 1 prerelease; 1 draft.
	relJSON := fmt.Sprintf(`[
		{"tag_name":"v1.0.0","prerelease":false,"draft":false,
		 "published_at":%q,
		 "assets":[{"name":"app.tar.gz.sigstore"}]},
		{"tag_name":"v1.0.0-rc1","prerelease":true,"draft":false,
		 "published_at":%q,"assets":[]},
		{"tag_name":"v0.9.0-draft","prerelease":false,"draft":true,
		 "published_at":%q,"assets":[]}
	]`, t1, t1, t1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, relJSON)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.ReleaseCount != 1 {
		t.Errorf("ReleaseCount = %d; want 1 (draft and prerelease excluded)", got.ReleaseCount)
	}
	if !got.HasSignedReleaseAssets {
		t.Error("HasSignedReleaseAssets should be true (.sigstore asset)")
	}
	if got.DaysSinceLastRelease != 10 {
		t.Errorf("DaysSinceLastRelease = %d; want 10", got.DaysSinceLastRelease)
	}
}

// ---------------------------------------------------------------------------
// License: nil license field → LicenseSPDX=""
// ---------------------------------------------------------------------------

func TestCollect_NilLicense(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	repoNoLicenseJSON := `{
		"full_name":"acme/widget","stargazers_count":0,"subscribers_count":0,
		"forks_count":0,"open_issues_count":0,"archived":false,"disabled":false,
		"pushed_at":"2026-06-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z",
		"default_branch":"main","license":null
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoNoLicenseJSON)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	// Minimal stub — the no-license test doesn't need full collect to pass.
	// Override community stub to avoid 500.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoNoLicenseJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv2.Close()
	_ = srv

	c := client(srv2)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.LicenseSPDX != "" {
		t.Errorf("LicenseSPDX = %q; want empty string when license is null", got.LicenseSPDX)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation aborts Collect and returns the context error
// ---------------------------------------------------------------------------

func TestCollect_ContextCancelled_Aborts(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Cancel the context immediately after the core repo call succeeds so that
	// the next endpoint (community/profile) sees a cancelled context.
	cancelCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
			// Signal the test that the first call succeeded, then cancel.
			close(cancelCh)
		default:
			// All subsequent endpoints: cancel is already in flight; the http
			// client will either not reach here or return a context error.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	// Cancel the context as soon as the repo call completes.
	go func() {
		<-cancelCh
		cancel()
	}()

	c := client(srv)
	_, err := Collect(ctx, c, "acme", "widget", now)
	// The error must be the context cancellation, not nil and not a Partial entry.
	if err == nil {
		t.Fatal("expected context error; got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context error; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 500 on a non-fatal endpoint degrades to Partial, does not abort
// ---------------------------------------------------------------------------

func TestCollect_ServerError500_DegradesToPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			// 500 — should degrade, not abort.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"internal error"}`))
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should not abort on 500 for community/profile; got: %v", err)
	}
	if !containsStr(got.Partial, "community_profile") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "community_profile")
	}
	// Core repo fields still populated.
	if got.Stars != 1500 {
		t.Errorf("Stars = %d; want 1500", got.Stars)
	}
}

// ---------------------------------------------------------------------------
// processWorkflows: one file 404, one file OK → WorkflowsFetched=true, not in Partial
// ---------------------------------------------------------------------------

func TestCollect_WorkflowOneFileFails_PartialFetchStillScans(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Two workflows: ci.yml (404), release.yml (OK, contains pull_request_target).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON) // ci.yml (active) + release.yml (disabled)
		// ci.yml fetch returns 404 — per-file failure, should not void the scan.
		case "/repos/acme/widget/contents/.github/workflows/ci.yml":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		// release.yml fetch returns content with pull_request_target.
		case "/repos/acme/widget/contents/.github/workflows/release.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releaseYAML)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be true (release.yml was fetched successfully)")
	}
	if !got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be true (release.yml contains the trigger)")
	}
	if containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; must NOT contain %q when at least one file was fetched", got.Partial, "workflow_safety")
	}
}

// ---------------------------------------------------------------------------
// processWorkflows: all file fetches fail → WorkflowsFetched=false, in Partial
// ---------------------------------------------------------------------------

func TestCollect_WorkflowAllFilesFail_WorkflowSafetyInPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON)
		// Both workflow file fetches fail.
		case "/repos/acme/widget/contents/.github/workflows/ci.yml",
			"/repos/acme/widget/contents/.github/workflows/release.yml":
			reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Reset", reset)
			w.WriteHeader(http.StatusForbidden)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be false when all file fetches fail")
	}
	if got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be false when no files could be fetched")
	}
	if !containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "workflow_safety")
	}
}

// ---------------------------------------------------------------------------
// collectCounts error paths: each of the three sub-calls can degrade
// ---------------------------------------------------------------------------

// countBaseRoutes returns routes for all endpoints except those under test, so
// sub-tests can overlay just the failing paths.
func countBaseRoutes(now time.Time) map[string]fixture {
	return map[string]fixture{
		"/repos/acme/widget":                       {200, repoJSON},
		"/repos/acme/widget/community/profile":     {200, communityJSON},
		"/repos/acme/widget/stats/contributors":    {200, `[]`},
		"/repos/acme/widget/stats/commit_activity": {200, `[]`},
		"/repos/acme/widget/releases":              {200, `[]`},
		"/repos/acme/widget/actions/workflows":     {200, `{"total_count":0,"workflows":[]}`},
		"/repos/acme/widget/pulls":                 {200, `[]`},
		"/repos/acme/widget/issues":                {200, `[]`},
		"/repos/acme/widget/issues/comments":       {200, `[]`},
	}
}

func TestCollect_CountByState_OpenIssuesError_DegradesToPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			// per_page=1 (CountByState) → rate limit; per_page=100 (RecentIssues) → empty
			if r.URL.Query().Get("per_page") == "1" {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Limit", "60")
				w.Header().Set("X-RateLimit-Reset", reset)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should degrade on issues count error; got: %v", err)
	}
	if !containsStr(got.Partial, "issue_count_open") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "issue_count_open")
	}
}

func TestCollect_PullRequestCounts_Error_DegradesToPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			// CountByState calls with per_page=1; RecentIssues with per_page=100
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			// per_page=1 (CountByState for PRs) → rate limit; per_page=100 (RecentPulls) → empty
			if r.URL.Query().Get("per_page") == "1" {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Limit", "60")
				w.Header().Set("X-RateLimit-Reset", reset)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should degrade on PR count error; got: %v", err)
	}
	if !containsStr(got.Partial, "pr_counts") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "pr_counts")
	}
}

// ---------------------------------------------------------------------------
// Multiple non-fatal 500 errors: contributor_stats, commit_activity, releases,
// workflows, closed_pulls, issue_ttfr all degrade without aborting.
// ---------------------------------------------------------------------------

func TestCollect_MultipleEndpoints500_AllDegrade(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		// These all 500 → must end up in Partial.
		case "/repos/acme/widget/stats/contributors",
			"/repos/acme/widget/stats/commit_activity",
			"/repos/acme/widget/releases",
			"/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"server error"}`))
		// issues + pulls: CountByState (per_page=1) return empty; RecentIssues/RecentPulls OK.
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":
			// per_page=1 → 500 (PR counts fail); per_page=100 (RecentPulls) → 500 (closed_pulls fail)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"server error"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should not abort on multiple non-fatal 500s; got: %v", err)
	}
	for _, want := range []string{
		"contributor_stats", "commit_activity", "releases", "workflows",
	} {
		if !containsStr(got.Partial, want) {
			t.Errorf("Partial = %v; want to contain %q", got.Partial, want)
		}
	}
}

// ---------------------------------------------------------------------------
// collectCounts: context abort mid-counts propagates as an error
// ---------------------------------------------------------------------------

func TestCollect_CollectCounts_ContextAbort(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track whether we've hit the workflows endpoint (step 6, just before counts).
	workflowsDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)
		case "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
			// Cancel the context after workflows — next is collectCounts.
			select {
			case <-workflowsDone:
			default:
				close(workflowsDone)
				cancel()
			}
		default:
			// All count endpoints will see a cancelled context.
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	// Wait for the cancel signal before letting Collect proceed past workflows.
	go func() { <-workflowsDone }()

	c := client(srv)
	_, err := Collect(ctx, c, "acme", "widget", now)
	if err == nil {
		t.Fatal("expected context error after cancellation at counts step; got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context error; got %v", err)
	}
}

// unused imports guard
var _ = errors.New
var _ = countBaseRoutes
