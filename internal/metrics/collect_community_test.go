package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Bot-filtering in TTFR: first comment by a bot should be skipped
// ---------------------------------------------------------------------------

func TestCollect_TTFRBotFiltered(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	issueCreated := now.Add(-20 * time.Hour)
	// First comment is a bot (2h after creation), second is human (5h after).
	// Bot-filtered TTFR should be 5h, not 2h.
	commentsJSON := buildIssueCommentsJSON(now, issueCreated, true /* firstBot */)

	oneIssueJSON := fmt.Sprintf(`[
		{"number":20,"state":"open","created_at":%q,"closed_at":null,
		 "comments":2,"user":{"login":"reporter","type":"User"},
		 "author_association":"NONE","pull_request":null}
	]`, issueCreated.UTC().Format(time.RFC3339))

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
			if strings.Contains(r.URL.RawQuery, "state=all") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oneIssueJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		case "/repos/acme/widget/issues/20/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, commentsJSON)
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
		t.Fatalf("Collect: %v", err)
	}
	// First human response is 5h after creation.
	if got.MedianIssueFirstResponseHours < 4.9 || got.MedianIssueFirstResponseHours > 5.1 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≈5.0 (bot filtered)", got.MedianIssueFirstResponseHours)
	}
}

// ---------------------------------------------------------------------------
// TTFR: first response must be by a different login than the issue author
// ---------------------------------------------------------------------------

func TestCollect_TTFRSameAuthorFiltered(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	issueCreated := now.Add(-10 * time.Hour)

	// Both comments are by the issue author themselves — no valid first response.
	selfComments := fmt.Sprintf(`[
		{"created_at":%q,"user":{"login":"reporter","type":"User"}},
		{"created_at":%q,"user":{"login":"reporter","type":"User"}}
	]`,
		issueCreated.Add(2*time.Hour).UTC().Format(time.RFC3339),
		issueCreated.Add(4*time.Hour).UTC().Format(time.RFC3339),
	)

	oneIssueJSON := fmt.Sprintf(`[
		{"number":30,"state":"open","created_at":%q,"closed_at":null,
		 "comments":2,"user":{"login":"reporter","type":"User"},
		 "author_association":"NONE","pull_request":null}
	]`, issueCreated.UTC().Format(time.RFC3339))

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
			if strings.Contains(r.URL.RawQuery, "state=all") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oneIssueJSON)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `[]`)
			}
		case "/repos/acme/widget/issues/30/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, selfComments)
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
		t.Fatalf("Collect: %v", err)
	}
	// No valid first response → MedianIssueFirstResponseHours ≤ 0
	if got.MedianIssueFirstResponseHours > 0 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≤0 (all comments by author)", got.MedianIssueFirstResponseHours)
	}
}

// ---------------------------------------------------------------------------
// Self-merge exclusion in newcomer PRs
// ---------------------------------------------------------------------------

func TestCollect_NewcomerSelfMergeExcluded(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	t1 := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// Single PR: newcomer FIRST_TIME_CONTRIBUTOR, merged by themselves.
	// Should produce 0 NewcomerPRsMerged.
	selfMergeJSON := fmt.Sprintf(`[
		{"number":5,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"newcomer","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR",
		 "merged_by":{"login":"newcomer","type":"User"}}
	]`, t1, t1, t1)

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
			if strings.Contains(r.URL.RawQuery, "state=closed") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, selfMergeJSON)
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
	if got.NewcomerPRsMerged != 0 {
		t.Errorf("NewcomerPRsMerged = %d; want 0 (self-merge excluded)", got.NewcomerPRsMerged)
	}
}

// ---------------------------------------------------------------------------
// Newcomer 90-day window: PR outside window excluded
// ---------------------------------------------------------------------------

func TestCollect_NewcomerOutsideWindow_Excluded(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	// PR closed 200 days ago — outside 90d window.
	tOld := now.Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339)

	oldPRJSON := fmt.Sprintf(`[
		{"number":6,"state":"closed","created_at":%q,"closed_at":%q,
		 "merged_at":%q,
		 "user":{"login":"alice","type":"User"},
		 "author_association":"FIRST_TIME_CONTRIBUTOR",
		 "merged_by":{"login":"maintainer","type":"User"}}
	]`, tOld, tOld, tOld)

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
			if strings.Contains(r.URL.RawQuery, "state=closed") && strings.Contains(r.URL.RawQuery, "per_page=100") {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oldPRJSON)
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
	if got.NewcomerPRsMerged != 0 {
		t.Errorf("NewcomerPRsMerged = %d; want 0 (PR outside 90d window)", got.NewcomerPRsMerged)
	}
}
