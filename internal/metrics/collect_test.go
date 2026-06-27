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
	if got.HasReadme || got.HasSecurityPolicy {
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
		// issues → OK (RecentIssues succeeds, issue cohort is computed).
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		// pulls → 500 for all calls: closed_pulls and pr_cohort both degrade.
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

// ---------------------------------------------------------------------------
// context abort mid-fanout propagates as an error
// ---------------------------------------------------------------------------

// TestCollect_FanOut_ContextAbort verifies that context cancellation after the
// repo gate causes all in-flight fan-out calls to abort and Collect to return
// a context error. Uses the deterministic repo-triggered-cancel pattern.
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

// ---------------------------------------------------------------------------
// pr_cohort graceful degradation: rate-limit on RecentPullsByCreation
// ---------------------------------------------------------------------------

// TestCollect_PRCohort_RateLimited_DegradesToPartial verifies that a
// rate-limit (or any non-context error) on the RecentPullsByCreation call
// records "pr_cohort" in Partial, leaves RecentPRsMerged/Open at zero
// (yielding a neutral 50 via ratioScore), and does not abort collection.
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
			// state=all & sort=created → RecentPullsByCreation (pr_cohort) → rate-limit.
			// state=closed & sort=updated → RecentPulls (closed_pulls) → OK.
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
	// Counts stay zero → neutral 50 via ratioScore.
	if got.RecentPRsMerged != 0 {
		t.Errorf("RecentPRsMerged = %d; want 0 when pr_cohort degraded", got.RecentPRsMerged)
	}
	if got.RecentPRsOpen != 0 {
		t.Errorf("RecentPRsOpen = %d; want 0 when pr_cohort degraded", got.RecentPRsOpen)
	}
}

// ---------------------------------------------------------------------------
// Cohort window exclusion: issues/PRs outside 90d are not counted
// ---------------------------------------------------------------------------

// TestIssueCreationCohort_WindowExclusion verifies that issues with CreatedAt
// older than the 90-day newcomerWindow are excluded from RecentIssuesClosed and
// RecentIssuesOpen, and that issues on the boundary are included.
func TestIssueCreationCohort_WindowExclusion(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Three issues: one within window (open), one outside window (closed), one
	// exactly at boundary (open). Only the in-window and boundary issues count.
	inWindow := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	outWindow := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)
	// Boundary: exactly 90 days ago is still within the window (not Before).
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
			// Both TTFR (state=all) and comments calls share this path;
			// return issues for any query — the cohort filter is what's under test.
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
	// issue 2 (closed, 200d ago) is outside the window → excluded.
	// issue 1 (open, 10d ago) and issue 3 (open, 90d ago boundary) are included.
	if got.RecentIssuesClosed != 0 {
		t.Errorf("RecentIssuesClosed = %d; want 0 (outside-window issue excluded)", got.RecentIssuesClosed)
	}
	if got.RecentIssuesOpen != 2 {
		t.Errorf("RecentIssuesOpen = %d; want 2 (in-window + boundary)", got.RecentIssuesOpen)
	}
}
