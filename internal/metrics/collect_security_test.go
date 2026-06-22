package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WorkflowsFetched=false when all workflow file fetches fail
// ---------------------------------------------------------------------------

func TestCollect_WorkflowFileFetchFails_WorkflowsFetchedFalse(t *testing.T) {
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
			_, _ = fmt.Fprint(w, workflowsJSON) // returns 2 workflows
		// All workflow file fetches return rate-limit
		case "/repos/acme/widget/contents/.github/workflows/ci.yml",
			"/repos/acme/widget/contents/.github/workflows/release.yml":
			rateLimitHandler()(w, r)
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
		t.Fatalf("Collect: %v", err)
	}
	if got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be false when file fetches fail")
	}
	if got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be false when files not fetched")
	}
	if !containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "workflow_safety")
	}
}

// ---------------------------------------------------------------------------
// No active workflows → HasCI=false
// ---------------------------------------------------------------------------

func TestCollect_NoActiveWorkflows_HasCIFalse(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	const noActiveWorkflowsJSON = `{
		"total_count":1,
		"workflows":[
			{"name":"Disabled","path":".github/workflows/disabled.yml","state":"disabled_manually"}
		]
	}`

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
			_, _ = fmt.Fprint(w, noActiveWorkflowsJSON)
		case "/repos/acme/widget/contents/.github/workflows/disabled.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, ciYAML)
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
		t.Fatalf("Collect: %v", err)
	}
	if got.HasCI {
		t.Error("HasCI should be false when no workflows are active")
	}
}

// ---------------------------------------------------------------------------
// processWorkflows: one file 404, one file OK → WorkflowsFetched=true, not in Partial
// ---------------------------------------------------------------------------

func TestCollect_WorkflowOneFileFails_PartialFetchStillScans(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	// Two workflows: ci.yml (404), release.yml (OK, contains pull_request_target).
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
			_, _ = fmt.Fprint(w, workflowsJSON) // ci.yml (active) + release.yml (disabled)
		// ci.yml fetch returns 404 — per-file failure, should not void the scan.
		case "/repos/acme/widget/contents/.github/workflows/ci.yml":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		// release.yml fetch returns content with pull_request_target.
		case "/repos/acme/widget/contents/.github/workflows/release.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releaseYAML)
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
		t.Fatalf("Collect: %v", err)
	}
	if !got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be true (release.yml was fetched successfully)")
	}
	if !got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be true (release.yml contains the trigger)")
	}
	if containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; must NOT contain %q when at least one file was fetched", got.Partial, "workflow_safety")
	}
}

// ---------------------------------------------------------------------------
// processWorkflows: all file fetches fail → WorkflowsFetched=false, in Partial
// ---------------------------------------------------------------------------

func TestCollect_WorkflowAllFilesFail_WorkflowSafetyInPartial(t *testing.T) {
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
			_, _ = fmt.Fprint(w, workflowsJSON)
		// Both workflow file fetches fail.
		case "/repos/acme/widget/contents/.github/workflows/ci.yml",
			"/repos/acme/widget/contents/.github/workflows/release.yml":
			reset := strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10)
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Reset", reset)
			w.WriteHeader(http.StatusForbidden)
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
		t.Fatalf("Collect: %v", err)
	}
	if got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be false when all file fetches fail")
	}
	if got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be false when no files could be fetched")
	}
	if !containsStr(got.Partial, "workflow_safety") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "workflow_safety")
	}
}
