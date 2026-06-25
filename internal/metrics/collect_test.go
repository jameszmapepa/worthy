package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jameszmapepa/worthy/internal/github"
)

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

	if got.HasReadme || got.HasSecurityPolicy {
		t.Error("governance fields should be false when community/profile is missing")
	}
}

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

func TestCollect_ContextCancelled_Aborts(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	cancelCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)

			close(cancelCh)
		default:

			<-r.Context().Done()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	go func() {
		<-cancelCh
		cancel()
	}()

	c := client(srv)
	_, err := Collect(ctx, c, "acme", "widget", now)

	if err == nil {
		t.Fatal("expected context error; got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context error; got %v", err)
	}
}

func TestCollect_ContextCancelled_MidFanOut(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	fanoutReadyCh := make(chan struct{})
	var fanoutOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		default:

			fanoutOnce.Do(func() { close(fanoutReadyCh) })

			<-r.Context().Done()

		}
	}))
	defer srv.Close()

	go func() {
		<-fanoutReadyCh
		cancel()
	}()

	c := client(srv)
	_, err := Collect(ctx, c, "acme", "widget", now)
	if err == nil {
		t.Fatal("expected context error from mid-fan-out cancellation; got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context.Canceled or DeadlineExceeded; got %v", err)
	}
}

func TestCollect_ServerError500_DegradesToPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/community/profile":

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

	if got.Stars != 1500 {
		t.Errorf("Stars = %d; want 1500", got.Stars)
	}
}

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

		case "/repos/acme/widget/stats/contributors",
			"/repos/acme/widget/stats/commit_activity",
			"/repos/acme/widget/commits",
			"/repos/acme/widget/releases",
			"/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"server error"}`))

		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)

		case "/repos/acme/widget/pulls":
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
		"closed_pulls", "pr_cohort",
	} {
		if !containsStr(got.Partial, want) {
			t.Errorf("Partial = %v; want to contain %q", got.Partial, want)
		}
	}
}

func TestCollect_FanOut_ContextAbort(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	cancelCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
			close(cancelCh)
		default:
			<-r.Context().Done()
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	go func() {
		<-cancelCh
		cancel()
	}()

	c := client(srv)
	_, err := Collect(ctx, c, "acme", "widget", now)
	if err == nil {
		t.Fatal("expected context error after cancellation; got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context error; got %v", err)
	}
}

func TestCollect_PRCohort_RateLimited_DegradesToPartial(t *testing.T) {
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
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/pulls":

			q := r.URL.Query()
			if q.Get("state") == "all" && q.Get("sort") == "created" {
				rateLimitHandler()(w, r)
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
		t.Fatalf("Collect should not abort on pr_cohort rate-limit; got: %v", err)
	}
	if !containsStr(got.Partial, "pr_cohort") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "pr_cohort")
	}

	if got.RecentPRsMerged != 0 {
		t.Errorf("RecentPRsMerged = %d; want 0 when pr_cohort degraded", got.RecentPRsMerged)
	}
	if got.RecentPRsOpen != 0 {
		t.Errorf("RecentPRsOpen = %d; want 0 when pr_cohort degraded", got.RecentPRsOpen)
	}
}

func TestIssueCreationCohort_WindowExclusion(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	inWindow := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	outWindow := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)

	boundary := now.Add(-90 * 24 * time.Hour).UTC().Format(time.RFC3339)

	issuesBody := fmt.Sprintf(`[
		{"number":1,"state":"open","created_at":%q,"closed_at":null,"comments":0,
		 "user":{"login":"alice","type":"User"}},
		{"number":2,"state":"closed","created_at":%q,"closed_at":%q,"comments":0,
		 "user":{"login":"bob","type":"User"}},
		{"number":3,"state":"open","created_at":%q,"closed_at":null,"comments":0,
		 "user":{"login":"carol","type":"User"}}
	]`, inWindow, outWindow, outWindow, boundary)

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
			_, _ = fmt.Fprint(w, issuesBody)
		case "/repos/acme/widget/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		case "/repos/acme/widget/issues/comments":
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

	if got.RecentIssuesClosed != 0 {
		t.Errorf("RecentIssuesClosed = %d; want 0 (outside-window issue excluded)", got.RecentIssuesClosed)
	}
	if got.RecentIssuesOpen != 2 {
		t.Errorf("RecentIssuesOpen = %d; want 2 (in-window + boundary)", got.RecentIssuesOpen)
	}
}
