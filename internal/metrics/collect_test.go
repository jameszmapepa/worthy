package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

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
			// Signal the test that the repo gate succeeded, then cancel.
			close(cancelCh)
		default:
			// Every fan-out endpoint blocks until the request context is
			// cancelled, so once cancel fires (post-repo) all in-flight calls
			// abort with a context error deterministically — regardless of
			// which collectors the bounded pool scheduled first.
			<-r.Context().Done()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	// Cancel the context as soon as the repo gate completes.
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

// TestCollect_ContextCancelled_MidFanOut proves that cancellation after the
// repository gate propagates through the errgroup fan-out workers and causes
// Collect to return a context error. This specifically exercises the
// g.Wait() → non-nil error path in Collect (collect.go:90-92).
//
// Design: fan-out endpoints acquire a semaphore slot then block UNTIL cancel
// fires. We guarantee cancel fires only after c.Repository has returned by
// waiting for fanoutReadyCh (signalled when the first fan-out request arrives
// at the server). This means at least one worker is mid-flight when cancel
// fires, so withCall / the HTTP call returns a context error, the worker
// returns it through the errgroup, and g.Wait() propagates it to Collect.
func TestCollect_ContextCancelled_MidFanOut(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// fanoutReadyCh is closed the first time a fan-out endpoint is hit.
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
			// Signal that at least one fan-out worker is in-flight.
			fanoutOnce.Do(func() { close(fanoutReadyCh) })
			// Block until the context is cancelled; the HTTP client then sees a
			// context error, which each worker propagates through the errgroup.
			<-r.Context().Done()
			// Writing after Done races with the client giving up; we write
			// nothing so the client always gets a context error.
		}
	}))
	defer srv.Close()

	// Cancel only after a fan-out worker is confirmed in-flight — guaranteeing
	// c.Repository has returned and the errgroup is running.
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

// TestCollect_CollectCounts_ContextAbort verifies the counts collector aborts
// with a context error when cancellation fires. The original premise (a strict
// workflows-then-counts ordering) no longer holds under concurrency, so this
// uses the deterministic repo-triggered-cancel pattern: the repo gate succeeds
// and cancels, and every fan-out endpoint blocks on the request context being
// done, so all in-flight calls (counts included) abort with a context error.
func TestCollect_CollectCounts_ContextAbort(t *testing.T) {
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

// ---------------------------------------------------------------------------
// collectCounts error paths: each of the three sub-calls can degrade
// ---------------------------------------------------------------------------

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
