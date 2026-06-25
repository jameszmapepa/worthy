package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jameszmapepa/worthy/internal/score"
)

func labelSearchServer(t *testing.T, openCount, availableCount, searchStatus int) *httptest.Server {
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

func TestCollect_NewcomerLabels_SecondCallFails(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

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

	if got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable = true; want false (availability unmeasured)")
	}

	if got.NewcomerLabeledOpen != 9 {
		t.Errorf("NewcomerLabeledOpen = %d; want 9 (first call succeeded)", got.NewcomerLabeledOpen)
	}
	if got.NewcomerLabeledAvailable != 0 {
		t.Errorf("NewcomerLabeledAvailable = %d; want 0 (second call failed)", got.NewcomerLabeledAvailable)
	}

	if !containsStr(got.Partial, "newcomer_labels") {
		t.Errorf("Partial = %v; want newcomer_labels (second search failed)", got.Partial)
	}

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
