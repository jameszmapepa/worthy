package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// Newcomer-friendliness is now measured repo-wide via the Search API
// (collectNewcomerLabels), not derived from the 100-newest-issues window: an
// "is:issue is:open label:..." search yields the total open beginner-labelled
// issues, and the same search with "no:assignee" yields the available subset.

// labelSearchServer builds a server that answers the newcomer-label searches
// with the given totals (open / unassigned) and serves repoJSON so Collect
// reaches the search step. All other endpoints return empty-but-valid bodies.
func labelSearchServer(t *testing.T, openCount, availableCount int, searchStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/widget":
			_, _ = fmt.Fprint(w, repoJSON)
		case r.URL.Path == "/search/issues":
			if searchStatus != http.StatusOK {
				w.WriteHeader(searchStatus)
				_, _ = fmt.Fprint(w, `{"message":"boom"}`)
				return
			}
			if strings.Contains(r.URL.Query().Get("q"), "no:assignee") {
				_, _ = fmt.Fprintf(w, `{"total_count":%d}`, availableCount)
			} else {
				_, _ = fmt.Fprintf(w, `{"total_count":%d}`, openCount)
			}
		case strings.HasSuffix(r.URL.Path, "/commits"),
			strings.HasSuffix(r.URL.Path, "/stats/commit_activity"),
			strings.HasSuffix(r.URL.Path, "/stats/contributors"),
			strings.HasSuffix(r.URL.Path, "/releases"),
			strings.HasSuffix(r.URL.Path, "/pulls"),
			strings.HasSuffix(r.URL.Path, "/issues"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.HasSuffix(r.URL.Path, "/actions/workflows"):
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case strings.HasSuffix(r.URL.Path, "/community/profile"):
			_, _ = fmt.Fprint(w, communityJSON)
		default:
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
}

// Repo-wide counts surface both the open total and the unassigned (available)
// subset, and the success flag is set.
func TestCollect_NewcomerLabels_OpenAndAvailable(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	srv := labelSearchServer(t, 5, 2, http.StatusOK)
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if !got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable = false; want true (search succeeded)")
	}
	if got.NewcomerLabeledOpen != 5 {
		t.Errorf("NewcomerLabeledOpen = %d; want 5", got.NewcomerLabeledOpen)
	}
	if got.NewcomerLabeledAvailable != 2 {
		t.Errorf("NewcomerLabeledAvailable = %d; want 2", got.NewcomerLabeledAvailable)
	}
	if containsStr(got.Partial, "newcomer_labels") {
		t.Errorf("Partial = %v; should not contain newcomer_labels on success", got.Partial)
	}
}

// No beginner labels at all is a clean, successful zero — not a degradation.
func TestCollect_NewcomerLabels_NoneIsSuccessfulZero(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	srv := labelSearchServer(t, 0, 0, http.StatusOK)
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if !got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable = false; want true (search succeeded, just zero hits)")
	}
	if got.NewcomerLabeledOpen != 0 || got.NewcomerLabeledAvailable != 0 {
		t.Errorf("counts = (%d,%d); want (0,0)", got.NewcomerLabeledOpen, got.NewcomerLabeledAvailable)
	}
}

// When the first search (total open count) succeeds but the second (no:assignee
// available count) fails, availability is UNKNOWN. We must not pretend the
// labelled issues are "all assigned": ok stays false so the scorer falls back to
// its neutral "label data unavailable" branch, and partial is set so the UI can
// flag incomplete data. The raw open total is still recorded for reference.
func TestCollect_NewcomerLabels_SecondCallFails(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// First search (total) succeeds; second (no:assignee) returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/acme/widget":
			_, _ = fmt.Fprint(w, repoJSON)
		case r.URL.Path == "/search/issues":
			if strings.Contains(r.URL.Query().Get("q"), "no:assignee") {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"message":"boom"}`)
			} else {
				_, _ = fmt.Fprintf(w, `{"total_count":9}`)
			}
		case strings.HasSuffix(r.URL.Path, "/commits"),
			strings.HasSuffix(r.URL.Path, "/stats/commit_activity"),
			strings.HasSuffix(r.URL.Path, "/stats/contributors"),
			strings.HasSuffix(r.URL.Path, "/releases"),
			strings.HasSuffix(r.URL.Path, "/pulls"),
			strings.HasSuffix(r.URL.Path, "/issues"):
			_, _ = fmt.Fprint(w, `[]`)
		case strings.HasSuffix(r.URL.Path, "/actions/workflows"):
			_, _ = fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
		case strings.HasSuffix(r.URL.Path, "/community/profile"):
			_, _ = fmt.Fprint(w, communityJSON)
		default:
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should degrade gracefully; got: %v", err)
	}

	// ok stays false: availability is unknown, so the scorer must treat the
	// signal as unavailable rather than claiming the issues are all assigned.
	if got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable = true; want false (availability unmeasured)")
	}
	// The open total we did get is still recorded for reference.
	if got.NewcomerLabeledOpen != 9 {
		t.Errorf("NewcomerLabeledOpen = %d; want 9 (first call succeeded)", got.NewcomerLabeledOpen)
	}
	if got.NewcomerLabeledAvailable != 0 {
		t.Errorf("NewcomerLabeledAvailable = %d; want 0 (second call failed)", got.NewcomerLabeledAvailable)
	}
	// Partial is set to signal incomplete data.
	if !containsStr(got.Partial, "newcomer_labels") {
		t.Errorf("Partial = %v; want newcomer_labels (second search failed)", got.Partial)
	}

	// And the scorer must surface the neutral "unavailable" verdict, not a
	// fabricated "all assigned" one.
	rep := score.Evaluate(got)
	var sub score.SubScore
	for _, c := range rep.Categories {
		for _, s := range c.Subs {
			if s.Key == "newcomer_signals" {
				sub = s
			}
		}
	}
	if !strings.Contains(sub.Raw, "unavailable") {
		t.Errorf("newcomer_signals Raw = %q; want the neutral unavailable note", sub.Raw)
	}
}

// A failed search degrades gracefully: counts stay zero, the unavailable flag is
// set so the scorer treats it as unknown (neutral), and Collect still succeeds.
func TestCollect_NewcomerLabels_SearchFailureDegrades(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	srv := labelSearchServer(t, 0, 0, http.StatusInternalServerError)
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should degrade gracefully; got: %v", err)
	}

	if got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable = true; want false when the search failed")
	}
	if !containsStr(got.Partial, "newcomer_labels") {
		t.Errorf("Partial = %v; want to contain newcomer_labels", got.Partial)
	}
}
