package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A2: Open PR ghosting — open PRs must be collected and contribute to
// OpenPRCount, MedianOpenPRAgeDays, and StaleNewcomerOpenPRs.

func TestCollect_OpenPRs_CountAndMedianAge(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Two open PRs: one 10 days old (MEMBER = not a newcomer), one 50 days old
	// (FIRST_TIME_CONTRIBUTOR = stale newcomer).
	t10 := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	t50 := now.Add(-50 * 24 * time.Hour).UTC().Format(time.RFC3339)

	openPRsJSON := fmt.Sprintf(`[
		{"number":1,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"alice","type":"User"},
		 "author_association":"MEMBER","merged_by":null},
		{"number":2,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"bob","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR","merged_by":null}
	]`, t10, t50)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/pulls":
			if q.Get("state") == "open" {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, openPRsJSON)
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

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if got.OpenPRCount != 2 {
		t.Errorf("OpenPRCount = %d; want 2", got.OpenPRCount)
	}

	// Median of [10, 50] days = (10+50)/2 = 30 days.
	if got.MedianOpenPRAgeDays < 29.9 || got.MedianOpenPRAgeDays > 30.1 {
		t.Errorf("MedianOpenPRAgeDays = %.2f; want ≈30.0", got.MedianOpenPRAgeDays)
	}

	// PR2 (FIRST_TIME_CONTRIBUTOR, 50 days old) qualifies as stale newcomer (>30d).
	// PR1 (MEMBER) does not qualify as a newcomer association.
	if got.StaleNewcomerOpenPRs != 1 {
		t.Errorf("StaleNewcomerOpenPRs = %d; want 1 (only FIRST_TIME_CONTRIBUTOR >30d)", got.StaleNewcomerOpenPRs)
	}
}

func TestCollect_OpenPRs_StaleNewcomerThreshold(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// NONE (newcomer) opened 29 days ago — NOT stale (must be > 30 days).
	t29 := now.Add(-29 * 24 * time.Hour).UTC().Format(time.RFC3339)
	// FIRST_TIMER opened 31 days ago — stale.
	t31 := now.Add(-31 * 24 * time.Hour).UTC().Format(time.RFC3339)

	openPRsJSON := fmt.Sprintf(`[
		{"number":3,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"carol","type":"User"},
		 "author_association":"NONE","merged_by":null},
		{"number":4,"state":"open","created_at":%q,"closed_at":null,
		 "merged_at":null,"user":{"login":"dave","type":"User"},
		 "author_association":"FIRST_TIMER","merged_by":null}
	]`, t29, t31)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/pulls":
			if q.Get("state") == "open" {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, openPRsJSON)
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

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// PR3 (NONE, 29d) is not stale (≤30 days); PR4 (FIRST_TIMER, 31d) is stale.
	if got.StaleNewcomerOpenPRs != 1 {
		t.Errorf("StaleNewcomerOpenPRs = %d; want 1 (only >30d newcomer)", got.StaleNewcomerOpenPRs)
	}
}

// A2: when the open-PR endpoint fails, degrade gracefully — open_pulls in
// Partial, open PR counts zero, collection continues.
func TestCollect_OpenPRs_RateLimited_DegradesToPartial(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/pulls":
			if q.Get("state") == "open" {
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

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should not abort on open_pulls rate-limit; got: %v", err)
	}
	if !containsStr(got.Partial, "open_pulls") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "open_pulls")
	}
	if got.OpenPRCount != 0 {
		t.Errorf("OpenPRCount = %d; want 0 when degraded", got.OpenPRCount)
	}
	if got.MedianOpenPRAgeDays != 0 {
		t.Errorf("MedianOpenPRAgeDays = %.2f; want 0 when degraded", got.MedianOpenPRAgeDays)
	}
	if got.StaleNewcomerOpenPRs != 0 {
		t.Errorf("StaleNewcomerOpenPRs = %d; want 0 when degraded", got.StaleNewcomerOpenPRs)
	}
}

// A2: verify the open-PR fetch uses state=open query parameter.
func TestCollect_OpenPRs_UsesStateOpenQuery(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// The httptest server dispatches each request in its own goroutine, so the
	// slice must be guarded to prevent a data race under -race.
	var (
		mu             sync.Mutex
		gotStateValues []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/repos/acme/widget/pulls" {
			mu.Lock()
			gotStateValues = append(gotStateValues, r.URL.Query().Get("state"))
			mu.Unlock()
		}
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	_, _ = Collect(context.Background(), client(srv), "acme", "widget", now)

	// At least one call must use state=open.
	mu.Lock()
	vals := append([]string(nil), gotStateValues...)
	mu.Unlock()

	found := false
	for _, s := range vals {
		if strings.EqualFold(s, "open") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pulls state values observed = %v; want at least one 'open' call", vals)
	}
}
