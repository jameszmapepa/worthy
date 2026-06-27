package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A3: newcomer-friendliness — GoodFirstIssues and HelpWantedIssues are derived
// from the already-fetched issue data (zero extra API calls), counting only
// open, non-PR issues carrying canonical label names.

func TestCollect_LabelCounts_GoodFirstAndHelpWanted(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	recent := now.Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// 4 issues:
	//  - #1 open, "good first issue" (counts toward GoodFirstIssues)
	//  - #2 open, "help wanted" (counts toward HelpWantedIssues)
	//  - #3 open, "Good-First-Issue" (variant casing — must also match)
	//  - #4 closed, "good first issue" (excluded: not open)
	//  - #5 open, pull_request field set (excluded: it's a PR)
	issuesJSON := fmt.Sprintf(`[
		{"number":1,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"a","type":"User"},
		 "author_association":"NONE","pull_request":null,
		 "labels":[{"name":"good first issue"}]},
		{"number":2,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"b","type":"User"},
		 "author_association":"NONE","pull_request":null,
		 "labels":[{"name":"help wanted"}]},
		{"number":3,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"c","type":"User"},
		 "author_association":"NONE","pull_request":null,
		 "labels":[{"name":"Good-First-Issue"}]},
		{"number":4,"state":"closed","created_at":%q,"closed_at":%q,
		 "comments":0,"user":{"login":"d","type":"User"},
		 "author_association":"NONE","pull_request":null,
		 "labels":[{"name":"good first issue"}]},
		{"number":5,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"e","type":"User"},
		 "author_association":"NONE","pull_request":{},
		 "labels":[{"name":"good first issue"}]}
	]`, recent, recent, recent, recent, recent, recent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issuesJSON)
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

	// Issues #1 and #3 are open, non-PR, and carry a "good first issue" variant.
	if got.GoodFirstIssues != 2 {
		t.Errorf("GoodFirstIssues = %d; want 2 (open non-PR issues with good-first label)", got.GoodFirstIssues)
	}
	// Issue #2 is open, non-PR, carries "help wanted".
	if got.HelpWantedIssues != 1 {
		t.Errorf("HelpWantedIssues = %d; want 1 (open non-PR issues with help-wanted label)", got.HelpWantedIssues)
	}
}

// A3: "help-wanted" (hyphenated) is equivalent to "help wanted" (space).
func TestCollect_LabelCounts_HyphenVariant(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	recent := now.Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339)

	issuesJSON := fmt.Sprintf(`[
		{"number":10,"state":"open","created_at":%q,"closed_at":null,
		 "comments":0,"user":{"login":"x","type":"User"},
		 "author_association":"NONE","pull_request":null,
		 "labels":[{"name":"help-wanted"},{"name":"good-first-issue"}]}
	]`, recent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issuesJSON)
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

	if got.GoodFirstIssues != 1 {
		t.Errorf("GoodFirstIssues = %d; want 1 (good-first-issue hyphenated variant)", got.GoodFirstIssues)
	}
	if got.HelpWantedIssues != 1 {
		t.Errorf("HelpWantedIssues = %d; want 1 (help-wanted hyphenated variant)", got.HelpWantedIssues)
	}
}

// A3: when issues are degraded (ttfr partial), label counts stay at zero —
// no extra API call is attempted.
func TestCollect_LabelCounts_ZeroWhenIssuesFail(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)
		case "/repos/acme/widget/issues":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"error"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect should degrade gracefully; got: %v", err)
	}
	if got.GoodFirstIssues != 0 {
		t.Errorf("GoodFirstIssues = %d; want 0 when issues fetch degraded", got.GoodFirstIssues)
	}
	if got.HelpWantedIssues != 0 {
		t.Errorf("HelpWantedIssues = %d; want 0 when issues fetch degraded", got.HelpWantedIssues)
	}
}
