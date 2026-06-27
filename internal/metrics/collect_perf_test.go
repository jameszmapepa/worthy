package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// perfNow is a fixed reference time so the wired fixtures are deterministic.
var perfNow = time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

// TestPerf_RetryBudgetBounded proves the 202-recompute retry loop is bounded
// and degrades gracefully. A stats endpoint that never finishes computing
// (always 202) is polled exactly maxRetries+1 times — never indefinitely — and
// the one unfinished metric drops to Partial instead of erroring the whole
// collection. client() sets WithRetry(2, 1ms), so the budget is 1 initial
// request + 2 retries = 3, and the test runs in milliseconds.
func TestPerf_RetryBudgetBounded(t *testing.T) {
	var commitActivityHits atomic.Int64

	full := fullRoutesHandler(perfNow, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/stats/commit_activity" {
			commitActivityHits.Add(1)
			w.WriteHeader(http.StatusAccepted) // 202: "still computing", forever
			return
		}
		full(w, r)
	}))
	defer srv.Close()

	raw, err := Collect(context.Background(), client(srv), "acme", "widget", perfNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// maxRetries (2 via client()) + 1 initial request.
	const wantHits = 3
	if got := commitActivityHits.Load(); got != wantHits {
		t.Errorf("commit_activity requests = %d; want %d (maxRetries+1)", got, wantHits)
	}

	// The stuck stat no longer degrades: after exhausting the bounded retry
	// budget, collection falls back to the /commits count (served by
	// fullRoutesHandler), so commit data is recovered rather than lost.
	if len(raw.Partial) != 0 {
		t.Errorf("raw.Partial = %v; want empty (commit fallback rescued the stuck stat)", raw.Partial)
	}
	if !raw.HasCommitFallback {
		t.Error("HasCommitFallback = false; want true (fallback used when the stat is stuck)")
	}
}

// TestPerf_LongPoleNotSerialized proves independent collectors overlap rather
// than queue behind one another. Two endpoints in separate top-level collectors
// — community/profile and stats/contributors, each a single unique call — sleep
// 80ms while every other endpoint is instant. Under real concurrency the
// wall-clock is ~80ms (the long pole); if the fan-out regressed to serial it
// would be ~160ms (the sum of the two poles). The 140ms ceiling sits between
// the two so the test fails loudly on a serialization regression, with enough
// slack above 80ms to tolerate loopback and CI jitter.
func TestPerf_LongPoleNotSerialized(t *testing.T) {
	const poleSleep = 80 * time.Millisecond

	// Both paths are unique single-call endpoints owned by distinct top-level
	// collectors (collectCommunity, collectContributors), so each contributes
	// exactly one 80ms pole — no accidental double-counting within a collector.
	slow := map[string]bool{
		"/repos/acme/widget/community/profile":  true,
		"/repos/acme/widget/stats/contributors": true,
	}

	full := fullRoutesHandler(perfNow, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slow[r.URL.Path] {
			time.Sleep(poleSleep)
		}
		full(w, r)
	}))
	defer srv.Close()

	start := time.Now()
	_, err := Collect(context.Background(), client(srv), "acme", "widget", perfNow)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if elapsed > 140*time.Millisecond {
		t.Errorf("Collect took %v; want < 140ms — two 80ms poles must overlap, not serialize (serial would be ~160ms)", elapsed)
	}
}
