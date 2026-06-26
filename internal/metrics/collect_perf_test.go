package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var perfNow = time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

// TestPerf_RetryBudgetBounded: client() sets WithRetry(2, 1ms) → 1 initial + 2 retries = 3 hits.
func TestPerf_RetryBudgetBounded(t *testing.T) {
	var commitActivityHits atomic.Int64

	full := fullRoutesHandler(perfNow, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/stats/commit_activity" {
			commitActivityHits.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		full(w, r)
	}))
	defer srv.Close()

	raw, err := Collect(context.Background(), client(srv), "acme", "widget", perfNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	const wantHits = 3
	if got := commitActivityHits.Load(); got != wantHits {
		t.Errorf("commit_activity requests = %d; want %d (maxRetries+1)", got, wantHits)
	}

	if len(raw.Partial) != 0 {
		t.Errorf("raw.Partial = %v; want empty (commit fallback rescued the stuck stat)", raw.Partial)
	}
	if !raw.HasCommitFallback {
		t.Error("HasCommitFallback = false; want true (fallback used when the stat is stuck)")
	}
}

// TestPerf_LongPoleNotSerialized: two 80ms poles must overlap (~80ms), not serialize (~160ms); 140ms ceiling sits between.
func TestPerf_LongPoleNotSerialized(t *testing.T) {
	const poleSleep = 80 * time.Millisecond

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
