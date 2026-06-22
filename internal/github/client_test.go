package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server, opts ...Option) *Client {
	base := make([]Option, 0, 3+len(opts))
	base = append(base,
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Millisecond),
	)
	return NewClient(append(base, opts...)...)
}

func jsonHandler(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func TestGet_HappyPath(t *testing.T) {
	type payload struct {
		Message string `json:"message"`
	}
	srv := httptest.NewServer(jsonHandler(http.StatusOK, payload{Message: "hello"}))
	defer srv.Close()

	c := newTestClient(srv)
	var got payload
	if err := c.get(context.Background(), "/test", &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Message != "hello" {
		t.Errorf("got message %q; want %q", got.Message, "hello")
	}
}

func TestGetWithHeader_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "sentinel")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	hdr, err := c.getWithHeader(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("getWithHeader: %v", err)
	}
	if hdr.Get("X-Custom") != "sentinel" {
		t.Errorf("header X-Custom = %q; want %q", hdr.Get("X-Custom"), "sentinel")
	}
}

func TestGet_202ThenSuccess(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"gopher"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	var u User
	if err := c.get(context.Background(), "/test", &u); err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if u.Login != "gopher" {
		t.Errorf("login = %q; want %q", u.Login, "gopher")
	}
	if calls != 2 {
		t.Errorf("server calls = %d; want 2", calls)
	}
}

func TestGet_202ExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := newTestClient(srv, WithRetry(2, time.Millisecond))
	err := c.get(context.Background(), "/stats", nil)
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	if !contains(err.Error(), "stats") {
		t.Errorf("error %q should mention path", err.Error())
	}
}

func TestGet_202FlatBackoff(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	const retries = 5
	const wait = 30 * time.Millisecond
	c := newTestClient(srv, WithRetry(retries, wait))

	start := time.Now()
	err := c.get(context.Background(), "/stats", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after exhausting 202 retries")
	}
	if calls != retries+1 {
		t.Errorf("server calls = %d; want %d (initial request + %d retries)", calls, retries+1, retries)
	}
	if maxFlat := retries*wait + 150*time.Millisecond; elapsed > maxFlat {
		t.Errorf("elapsed %v exceeds flat budget %v; backoff appears to be escalating", elapsed, maxFlat)
	}
}

func TestGet_202ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := newTestClient(srv, WithRetry(100, 50*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := c.get(ctx, "/slow-stats", nil)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestGet_404ReturnsNotFoundError(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(http.StatusNotFound, map[string]string{"message": "Not Found"}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/repos/no/such", nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("want *NotFoundError; got %T: %v", err, err)
	}
	if nfe.Endpoint != "/repos/no/such" {
		t.Errorf("NotFoundError.Endpoint = %q; want %q", nfe.Endpoint, "/repos/no/such")
	}
}

func TestGet_403RateLimited(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.Header().Set("X-RateLimit-Resource", "core")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/repos/o/r", nil)
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("want *RateLimitError; got %T: %v", err, err)
	}
	if rle.Limit != 60 {
		t.Errorf("Limit = %d; want 60", rle.Limit)
	}
	if rle.Reset.Unix() != reset {
		t.Errorf("Reset = %v; want unix %d", rle.Reset, reset)
	}
	if rle.Endpoint != "/repos/o/r" {
		t.Errorf("Endpoint = %q; want %q", rle.Endpoint, "/repos/o/r")
	}
}

func TestGet_429RateLimited(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/repos/o/r", nil)
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("want *RateLimitError for 429; got %T: %v", err, err)
	}
	if rle.Limit != 5000 {
		t.Errorf("Limit = %d; want 5000", rle.Limit)
	}
}

func TestGet_403WithoutRemainingZeroIsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "50")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Must have push access"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/repo/o/r/settings", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var rle *RateLimitError
	if errors.As(err, &rle) {
		t.Error("got *RateLimitError for 403 without remaining:0; want generic error")
	}
}

func TestGet_403MissingRateLimitHeaderIsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"resource not accessible by integration"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/private", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var rle *RateLimitError
	if errors.As(err, &rle) {
		t.Error("got *RateLimitError for 403 without remaining header; want generic error")
	}
}

func TestWithToken_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithToken("ghp_testtoken"),
	)
	_ = c.get(context.Background(), "/test", nil)
	want := "Bearer ghp_testtoken"
	if gotAuth != want {
		t.Errorf("Authorization header = %q; want %q", gotAuth, want)
	}
}

func TestNoToken_SendsNoAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithToken(""),
	)
	_ = c.get(context.Background(), "/test", nil)
	if gotAuth != "" {
		t.Errorf("expected no Authorization header; got %q", gotAuth)
	}
}

func TestAuthenticated_ReflectsToken(t *testing.T) {
	withToken := NewClient(WithToken("tok"))
	if !withToken.Authenticated() {
		t.Error("Authenticated() = false; want true with token")
	}

	noToken := NewClient(WithToken(""))
	if noToken.Authenticated() {
		t.Error("Authenticated() = true; want false without token")
	}
}

func TestRepository_DecodeShape(t *testing.T) {
	body := `{
		"full_name":"acme/widget",
		"stargazers_count":1234,
		"subscribers_count":56,
		"forks_count":78,
		"open_issues_count":9,
		"pushed_at":"2025-01-15T12:00:00Z",
		"created_at":"2020-06-01T00:00:00Z",
		"default_branch":"main",
		"archived":false,
		"disabled":false,
		"fork":false,
		"license":{"spdx_id":"MIT","name":"MIT License"}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	r, err := c.Repository(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	if r.FullName != "acme/widget" {
		t.Errorf("FullName = %q; want %q", r.FullName, "acme/widget")
	}
	if r.Stargazers != 1234 {
		t.Errorf("Stargazers = %d; want 1234", r.Stargazers)
	}
	if r.License == nil || r.License.SPDXID != "MIT" {
		t.Errorf("License = %+v; want SPDXID=MIT", r.License)
	}
}

func TestRepository_EmptyOwnerRepo(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(http.StatusOK, map[string]any{}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Repository(context.Background(), "", "")
	if !errors.Is(err, errEmptyOwnerRepo) {
		t.Errorf("want errEmptyOwnerRepo; got %v", err)
	}

	_, err = c.Repository(context.Background(), "owner", "")
	if !errors.Is(err, errEmptyOwnerRepo) {
		t.Errorf("empty repo: want errEmptyOwnerRepo; got %v", err)
	}

	_, err = c.Repository(context.Background(), "", "repo")
	if !errors.Is(err, errEmptyOwnerRepo) {
		t.Errorf("empty owner: want errEmptyOwnerRepo; got %v", err)
	}
}

func TestCommunityProfile_DecodeShape(t *testing.T) {
	body := `{
		"health_percentage":80,
		"files":{
			"readme":{"url":"https://github.com"},
			"contributing":null,
			"license":{"url":"https://github.com"},
			"code_of_conduct":null,
			"security":{"url":"https://github.com"}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	p, err := c.CommunityProfile(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("CommunityProfile: %v", err)
	}
	if p.HealthPercentage != 80 {
		t.Errorf("HealthPercentage = %d; want 80", p.HealthPercentage)
	}
	if p.Files.Readme == nil {
		t.Error("Files.Readme should be non-nil")
	}
	if p.Files.Contributing != nil {
		t.Error("Files.Contributing should be nil")
	}
	if p.Files.SecurityPol == nil {
		t.Error("Files.SecurityPol should be non-nil")
	}
}

func TestContributorStats_DecodeShape(t *testing.T) {
	body := `[
		{"total":42,"author":{"login":"alice","type":"User"},
		 "weeks":[{"w":1700000000,"a":10,"d":2,"c":3}]}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	stats, err := c.ContributorStats(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ContributorStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d; want 1", len(stats))
	}
	s := stats[0]
	if s.Total != 42 {
		t.Errorf("Total = %d; want 42", s.Total)
	}
	if s.Author.Login != "alice" {
		t.Errorf("Author.Login = %q; want %q", s.Author.Login, "alice")
	}
	if len(s.Weeks) != 1 || s.Weeks[0].Commits != 3 {
		t.Errorf("Weeks[0].Commits = %d; want 3", s.Weeks[0].Commits)
	}
}

func TestReleases_DecodeShape(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	nowStr := now.Format(time.RFC3339)
	body := fmt.Sprintf(`[
		{"tag_name":"v1.2.3","prerelease":false,"draft":false,
		 "published_at":%q,
		 "assets":[{"name":"app.tar.gz"},{"name":"app.tar.gz.asc"}]}
	]`, nowStr)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rs, err := c.Releases(context.Background(), "acme", "widget", 10)
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("len(releases) = %d; want 1", len(rs))
	}
	rel := rs[0]
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q; want %q", rel.TagName, "v1.2.3")
	}
	if len(rel.Assets) != 2 {
		t.Errorf("len(Assets) = %d; want 2", len(rel.Assets))
	}
	if rel.Assets[1].Name != "app.tar.gz.asc" {
		t.Errorf("Assets[1].Name = %q; want %q", rel.Assets[1].Name, "app.tar.gz.asc")
	}
}

func TestWorkflows_DecodeShape(t *testing.T) {
	body := `{
		"total_count":2,
		"workflows":[
			{"name":"CI","path":".github/workflows/ci.yml","state":"active"},
			{"name":"Release","path":".github/workflows/release.yml","state":"disabled_manually"}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	wfs, err := c.Workflows(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("Workflows: %v", err)
	}
	if len(wfs) != 2 {
		t.Fatalf("len(workflows) = %d; want 2", len(wfs))
	}
	if wfs[0].State != "active" {
		t.Errorf("Workflows[0].State = %q; want %q", wfs[0].State, "active")
	}
}

func TestRecentPulls_DecodeShape(t *testing.T) {
	t1 := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	t2 := time.Now().UTC().Truncate(time.Second)
	body := fmt.Sprintf(`[
		{"number":5,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,"user":{"login":"bob","type":"User"},
		 "author_association":"CONTRIBUTOR","merged_by":{"login":"alice","type":"User"}},
		{"number":6,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"carol","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR","merged_by":null}
	]`, t1.Format(time.RFC3339), t2.Format(time.RFC3339), t2.Format(time.RFC3339), t1.Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	prs, err := c.RecentPulls(context.Background(), "acme", "widget", "closed")
	if err != nil {
		t.Fatalf("RecentPulls: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d; want 2", len(prs))
	}
	if !prs[0].IsMerged() {
		t.Error("prs[0].IsMerged() = false; want true (MergedAt is set)")
	}
	if prs[1].IsMerged() {
		t.Error("prs[1].IsMerged() = true; want false (MergedAt is nil)")
	}
}

func TestRecentPullsByCreation_DecodeShape(t *testing.T) {
	t1 := time.Now().Add(-10 * 24 * time.Hour).UTC().Truncate(time.Second)
	body := fmt.Sprintf(`[
		{"number":7,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,"user":{"login":"eve","type":"User"},
		 "author_association":"CONTRIBUTOR","merged_by":{"login":"alice","type":"User"}},
		{"number":8,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"frank","type":"User"},
		 "author_association":"NONE","merged_by":null}
	]`, t1.Format(time.RFC3339), t1.Format(time.RFC3339), t1.Format(time.RFC3339), t1.Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "all" {
			http.Error(w, "want state=all", http.StatusBadRequest)
			return
		}
		if q.Get("sort") != "created" {
			http.Error(w, "want sort=created", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	prs, err := c.RecentPullsByCreation(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("RecentPullsByCreation: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d; want 2", len(prs))
	}
	if !prs[0].IsMerged() {
		t.Error("prs[0].IsMerged() = false; want true")
	}
	if prs[1].IsMerged() {
		t.Error("prs[1].IsMerged() = true; want false")
	}
	if prs[1].State != "open" {
		t.Errorf("prs[1].State = %q; want open", prs[1].State)
	}
}

func TestRecentIssues_DecodeShape(t *testing.T) {
	t1 := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)
	body := fmt.Sprintf(`[
		{"number":10,"state":"open","created_at":%q,"closed_at":null,
		 "comments":3,"user":{"login":"dave","type":"User"},
		 "author_association":"NONE","pull_request":null},
		{"number":11,"state":"closed","created_at":%q,"closed_at":%q,
		 "comments":1,"user":{"login":"eve","type":"User"},
		 "author_association":"MEMBER","pull_request":{}}
	]`, t1.Format(time.RFC3339), t1.Format(time.RFC3339), t1.Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	issues, err := c.RecentIssues(context.Background(), "acme", "widget", "all")
	if err != nil {
		t.Fatalf("RecentIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d; want 2", len(issues))
	}
	if issues[0].IsPullRequest() {
		t.Error("issues[0].IsPullRequest() = true; want false (pull_request is null)")
	}
	if !issues[1].IsPullRequest() {
		t.Error("issues[1].IsPullRequest() = false; want true (pull_request is {})")
	}
}

func TestIssueComments_DecodeShape(t *testing.T) {
	t1 := time.Now().UTC().Truncate(time.Second)
	body := fmt.Sprintf(`[
		{"created_at":%q,"user":{"login":"frank","type":"User"}},
		{"created_at":%q,"user":{"login":"ci-bot[bot]","type":"Bot"}}
	]`, t1.Format(time.RFC3339), t1.Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cs, err := c.IssueComments(context.Background(), "acme", "widget", 42)
	if err != nil {
		t.Fatalf("IssueComments: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("len(comments) = %d; want 2", len(cs))
	}
	if cs[0].User.Login != "frank" {
		t.Errorf("cs[0].User.Login = %q; want %q", cs[0].User.Login, "frank")
	}
	if cs[1].User.Type != "Bot" {
		t.Errorf("cs[1].User.Type = %q; want %q", cs[1].User.Type, "Bot")
	}
}

func TestIssueIsPullRequest(t *testing.T) {
	issue := Issue{Number: 1}
	if issue.IsPullRequest() {
		t.Error("Issue with nil PullRequest should return false")
	}

	pr := Issue{Number: 2, PullRequest: &struct{}{}}
	if !pr.IsPullRequest() {
		t.Error("Issue with non-nil PullRequest should return true")
	}
}

func TestPullRequestIsMerged(t *testing.T) {
	open := PullRequest{Number: 1}
	if open.IsMerged() {
		t.Error("PR with nil MergedAt should not be merged")
	}

	merged := time.Now()
	closed := PullRequest{Number: 2, MergedAt: &merged}
	if !closed.IsMerged() {
		t.Error("PR with non-nil MergedAt should be merged")
	}
}

func TestClientSetsExpectedHeaders(t *testing.T) {
	var gotAccept, gotVersion, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotVersion = r.Header.Get("X-GitHub-Api-Version")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, WithToken(""))
	_ = c.get(context.Background(), "/test", nil)

	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q; want %q", gotAccept, "application/vnd.github+json")
	}
	if gotVersion != apiVersion {
		t.Errorf("X-GitHub-Api-Version = %q; want %q", gotVersion, apiVersion)
	}
	if gotUA != userAgent {
		t.Errorf("User-Agent = %q; want %q", gotUA, userAgent)
	}
}

func TestCommitActivity_DecodeShape(t *testing.T) {
	body := `[
		{"total":5,"week":1700000000},
		{"total":12,"week":1700604800}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	weeks, err := c.CommitActivity(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("CommitActivity: %v", err)
	}
	if len(weeks) != 2 {
		t.Fatalf("len(weeks) = %d; want 2", len(weeks))
	}
	if weeks[0].Total != 5 {
		t.Errorf("weeks[0].Total = %d; want 5", weeks[0].Total)
	}
	if weeks[1].Total != 12 {
		t.Errorf("weeks[1].Total = %d; want 12", weeks[1].Total)
	}
}

func TestRateLimitError_ErrorString(t *testing.T) {
	reset := time.Now().Add(5 * time.Minute)
	e := &RateLimitError{
		Reset:    reset,
		Limit:    60,
		Endpoint: "/repos/o/r",
		Resource: "core",
	}
	s := e.Error()
	if !contains(s, "60") {
		t.Errorf("error string %q should contain limit 60", s)
	}
	if !contains(s, "/repos/o/r") {
		t.Errorf("error string %q should contain endpoint", s)
	}
}

func TestNotFoundError_ErrorString(t *testing.T) {
	e := &NotFoundError{Endpoint: "/repos/missing/repo"}
	s := e.Error()
	if !contains(s, "/repos/missing/repo") {
		t.Errorf("error string %q should contain endpoint", s)
	}
}

func TestReleases_ClampPageZero(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	rs, err := c.Releases(context.Background(), "acme", "widget", 0)
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(rs) != 0 {
		t.Errorf("expected empty slice; got %d items", len(rs))
	}
	if !contains(gotQuery, "per_page=1") {
		t.Errorf("query %q; want per_page=1 (clamped from 0)", gotQuery)
	}
}

func TestReleases_ClampPageOverMax(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Releases(context.Background(), "acme", "widget", 200)
	if err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if !contains(gotQuery, "per_page=100") {
		t.Errorf("query %q; want per_page=100 (clamped from 200)", gotQuery)
	}
}

func TestGet_LongBodyTruncatedInError(t *testing.T) {
	longBody := make([]byte, 300)
	for i := range longBody {
		longBody[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(longBody)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	err := c.get(context.Background(), "/test", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !contains(err.Error(), "...") {
		t.Errorf("expected truncated body with '...' in error; got: %q", err.Error())
	}
}

func TestCommunityProfile_NotFound(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(http.StatusNotFound, map[string]string{"message": "Not Found"}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.CommunityProfile(context.Background(), "acme", "fork")
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("want *NotFoundError; got %T: %v", err, err)
	}
}

func TestContributorStats_NotFound(t *testing.T) {
	srv := httptest.NewServer(jsonHandler(http.StatusNotFound, map[string]string{"message": "Not Found"}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ContributorStats(context.Background(), "no", "such")
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("want *NotFoundError; got %T: %v", err, err)
	}
}

func TestRepository_PathEscapesOwnerAndRepo(t *testing.T) {
	var gotRequestURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)

	_, _ = c.Repository(context.Background(), "org name", "repo name")
	if !contains(gotRequestURI, "%20") {
		t.Errorf("RequestURI %q: want percent-encoded space (%%20) for owner/repo with spaces", gotRequestURI)
	}
}

func TestFileContent_PathEscapesSegments(t *testing.T) {
	var gotRequestURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	c := newTestClient(srv)

	_, _ = c.FileContent(context.Background(), "acme", "widget", ".github/workflows/my workflow.yml")

	if !contains(gotRequestURI, "my%20workflow.yml") {
		t.Errorf("RequestURI %q: want 'my%%20workflow.yml' for segment with space", gotRequestURI)
	}

	if !contains(gotRequestURI, ".github/workflows/") {
		t.Errorf("RequestURI %q: want literal '/' preserved between segments", gotRequestURI)
	}
}

func TestFileContent_HappyPath(t *testing.T) {
	wantBody := "on:\n  pull_request:\n    branches: [main]\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.raw+json" {
			t.Errorf("Accept = %q; want raw+json", r.Header.Get("Accept"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, wantBody)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("FileContent: %v", err)
	}
	if string(got) != wantBody {
		t.Errorf("body = %q; want %q", string(got), wantBody)
	}
}

func TestFileContent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/missing.yml")
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("want *NotFoundError; got %T: %v", err, err)
	}
}

func TestFileContent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/ci.yml")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var nfe *NotFoundError
	if errors.As(err, &nfe) {
		t.Error("500 should not be a NotFoundError")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
