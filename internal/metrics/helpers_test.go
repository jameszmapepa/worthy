package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/jameszmapepa/worthy/internal/github"
)

type fixture struct {
	status int
	body   string
}

// mux: unregistered paths return 500 so tests fail loudly on unexpected Collect endpoints.
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

func client(srv *httptest.Server) *github.Client {
	return github.NewClient(
		github.WithBaseURL(srv.URL),
		github.WithHTTPClient(srv.Client()),
		github.WithRetry(2, time.Millisecond),
		github.WithToken(""),
	)
}

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

const releaseYAML = `
on:
  pull_request_target:
    branches: [main]
jobs:
  release:
    runs-on: ubuntu-latest
`

func buildClosedPRsJSON(now time.Time) string {
	t := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	tOld := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)
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
		t, t, t,
		t, t,
		t, t, t,
		tOld, tOld, tOld,
	)
}

const openPRsLinkJSON = `[{"number":99,"state":"open"}]`

const closedPRsLinkJSON = `[{"number":98,"state":"closed"}]`

const openIssuesLinkJSON = `[{"number":50,"state":"open"}]`

const closedIssuesLinkJSON = `[{"number":51,"state":"closed"}]`

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

func buildIssueCommentsJSON(now, issueCreated time.Time, firstBot bool) string {
	t1 := issueCreated.Add(2 * time.Hour).UTC().Format(time.RFC3339)
	t2 := issueCreated.Add(5 * time.Hour).UTC().Format(time.RFC3339)
	_ = now

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

func rateLimitFixture() fixture {
	reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
	return fixture{
		status: http.StatusForbidden,
		body: `{"message":"API rate limit exceeded","documentation_url":"x"}` + "\n" +
			fmt.Sprintf(`x-ratelimit-remaining: 0; reset: %s`, reset),
	}
}

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

func notFoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}
}

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

		"/repos/acme/widget/commits": {200, `[]`},
		"/search/issues":             {200, `{"total_count":0}`},
	}
}

var (
	_ = containsAll
	_ = rateLimitFixture
	_ = mux
	_ = countBaseRoutes
)
