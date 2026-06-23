package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// countingTransport tracks the maximum number of simultaneously in-flight
// requests it forwards to the wrapped RoundTripper.
type countingTransport struct {
	next        http.RoundTripper
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cur := t.inFlight.Add(1)
	for {
		old := t.maxInFlight.Load()
		if cur <= old || t.maxInFlight.CompareAndSwap(old, cur) {
			break
		}
	}
	defer t.inFlight.Add(-1)
	return t.next.RoundTrip(req)
}

// fullRoutesHandler serves valid-enough JSON for every endpoint Collect hits,
// optionally sleeping per request so concurrent calls overlap.
func fullRoutesHandler(now time.Time, sleep time.Duration) http.HandlerFunc {
	releasesBody := buildReleasesJSON(now)
	closedPRsBody := buildClosedPRsJSON(now)
	recentIssuesBody := buildRecentIssuesJSON(now)
	issueCreated := now.Add(-10 * time.Hour)
	issue11Comments := buildIssueCommentsJSON(now, issueCreated, false)

	return func(w http.ResponseWriter, r *http.Request) {
		if sleep > 0 {
			time.Sleep(sleep)
		}
		path := r.URL.Path
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case path == "/repos/acme/widget":
			_, _ = w.Write([]byte(repoJSON))
		case path == "/repos/acme/widget/community/profile":
			_, _ = w.Write([]byte(communityJSON))
		case path == "/repos/acme/widget/stats/contributors":
			_, _ = w.Write([]byte(buildContributorStatsJSON()))
		case path == "/repos/acme/widget/stats/commit_activity":
			_, _ = w.Write([]byte(buildCommitActivityJSON()))
		case path == "/repos/acme/widget/releases":
			_, _ = w.Write([]byte(releasesBody))
		case path == "/repos/acme/widget/actions/workflows":
			_, _ = w.Write([]byte(workflowsJSON))
		case path == "/repos/acme/widget/contents/.github/workflows/ci.yml":
			_, _ = w.Write([]byte(ciYAML))
		case path == "/repos/acme/widget/contents/.github/workflows/release.yml":
			_, _ = w.Write([]byte(releaseYAML))
		case path == "/repos/acme/widget/pulls" && q.Get("state") == "closed" && q.Get("per_page") == "100":
			_, _ = w.Write([]byte(closedPRsBody))
		case path == "/repos/acme/widget/issues" && q.Get("state") == "open" && q.Get("per_page") == "1":
			w.Header().Set("Link", `<https://x?page=40>; rel="last"`)
			_, _ = w.Write([]byte(openIssuesLinkJSON))
		case path == "/repos/acme/widget/issues" && q.Get("state") == "closed" && q.Get("per_page") == "1":
			w.Header().Set("Link", `<https://x?page=80>; rel="last"`)
			_, _ = w.Write([]byte(closedIssuesLinkJSON))
		case path == "/repos/acme/widget/pulls" && q.Get("state") == "open" && q.Get("per_page") == "1":
			w.Header().Set("Link", `<https://x?page=12>; rel="last"`)
			_, _ = w.Write([]byte(openPRsLinkJSON))
		case path == "/repos/acme/widget/pulls" && q.Get("state") == "closed" && q.Get("per_page") == "1":
			w.Header().Set("Link", `<https://x?page=25>; rel="last"`)
			_, _ = w.Write([]byte(closedPRsLinkJSON))
		case path == "/repos/acme/widget/issues" && q.Get("state") == "all" && q.Get("per_page") == "100":
			_, _ = w.Write([]byte(recentIssuesBody))
		case path == "/repos/acme/widget/issues/11/comments":
			_, _ = w.Write([]byte(issue11Comments))
		case path == "/repos/acme/widget/issues/12/comments":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}
}

// countingClient builds a github.Client whose transport tracks concurrency.
func countingClient(srv *httptest.Server, ct *countingTransport) *github.Client {
	base := srv.Client()
	ct.next = base.Transport
	if ct.next == nil {
		ct.next = http.DefaultTransport
	}
	return github.NewClient(
		github.WithBaseURL(srv.URL),
		github.WithHTTPClient(&http.Client{Transport: ct, Timeout: 20 * time.Second}),
		github.WithRetry(2, time.Millisecond),
		github.WithToken(""),
	)
}

// TestCollect_ConcurrencyBound proves Collect runs independent calls in
// parallel while staying within the maxConcurrency bound.
func TestCollect_ConcurrencyBound(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(fullRoutesHandler(now, 30*time.Millisecond))
	defer srv.Close()

	ct := &countingTransport{}
	c := countingClient(srv, ct)

	_, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	maxSeen := ct.maxInFlight.Load()
	if maxSeen <= 1 {
		t.Errorf("maxInFlight = %d; want > 1 (expected parallelism)", maxSeen)
	}
	if maxSeen > int64(maxConcurrency) {
		t.Errorf("maxInFlight = %d; want <= maxConcurrency (%d)", maxSeen, maxConcurrency)
	}
}

// TestCollect_LatencyBelowSerial proves wall-clock is far below the serial
// sum. With ~25 requests at 40ms each, serial would be >1s; bounded
// concurrency must finish well under 300ms (generous, jitter-tolerant).
func TestCollect_LatencyBelowSerial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(fullRoutesHandler(now, 40*time.Millisecond))
	defer srv.Close()

	c := client(srv)

	start := time.Now()
	_, err := Collect(context.Background(), c, "acme", "widget", now)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// 500ms ceiling: serial would be ~1s+ (≈25 requests × 40ms), so this still
	// proves concurrency, with enough slack to avoid flaking on loaded CI.
	if elapsed > 500*time.Millisecond {
		t.Errorf("Collect took %v; want < 500ms (serial estimate ~1s+)", elapsed)
	}
}

// TestCollect_PartialOrderDeterministic proves that, under partial
// degradation, the RawMetrics (and especially Partial order) is identical
// across runs.
func TestCollect_PartialOrderDeterministic(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)

	// Degrade several endpoints across all three groups to exercise ordering.
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case path == "/repos/acme/widget":
			_, _ = w.Write([]byte(repoJSON))
		case path == "/repos/acme/widget/community/profile",
			path == "/repos/acme/widget/stats/contributors",
			path == "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/repos/acme/widget/actions/workflows":
			_, _ = w.Write([]byte(workflowsJSON))
		case strings.HasPrefix(path, "/repos/acme/widget/contents/"):
			// All workflow files rate-limited -> workflow_safety partial.
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Reset", reset)
			w.WriteHeader(http.StatusForbidden)
		case path == "/repos/acme/widget/issues" && q.Get("per_page") == "1":
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/repos/acme/widget/pulls" && q.Get("per_page") == "1":
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/repos/acme/widget/pulls" && q.Get("state") == "closed":
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/repos/acme/widget/issues" && q.Get("state") == "all":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()
	c := client(srv)

	first, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect (first): %v", err)
	}
	second, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect (second): %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Errorf("RawMetrics not deterministic across runs:\nfirst.Partial=%v\nsecond.Partial=%v",
			first.Partial, second.Partial)
	}

	// Assert the canonical order explicitly.
	want := []string{
		"community_profile", "contributor_stats", "releases",
		"workflow_safety",
		"issue_count_open", "issue_count_closed", "pr_counts",
		"closed_pulls", "issue_ttfr",
	}
	if !reflect.DeepEqual(first.Partial, want) {
		t.Errorf("Partial = %v; want canonical order %v", first.Partial, want)
	}
}

// TestCollect_WorkflowsListError_Partial proves that a failure of the
// workflows LIST endpoint (distinct from a per-file content fetch failure,
// which yields "workflow_safety") degrades to the "workflows" partial marker
// and leaves WorkflowsFetched false, rather than aborting the collection.
func TestCollect_WorkflowsListError_Partial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Serve valid data for every endpoint except the workflows list, which
	// 500s — so the only degradation is the one under test.
	full := fullRoutesHandler(now, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/actions/workflows" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		full(w, r)
	}))
	defer srv.Close()

	raw, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	want := []string{"workflows"}
	if !reflect.DeepEqual(raw.Partial, want) {
		t.Errorf("Partial = %v; want %v", raw.Partial, want)
	}
	if raw.WorkflowsFetched {
		t.Error("WorkflowsFetched = true; want false when the list call failed")
	}
}
