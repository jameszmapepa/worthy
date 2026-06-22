package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Graceful degradation — rate limit on contributor stats
// ---------------------------------------------------------------------------

func TestCollect_ContributorStatsRateLimited_RecordedInPartial(t *testing.T) {
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
			rateLimitHandler()(w, r)
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
		t.Fatalf("Collect should not fail when contributor stats is rate-limited; got: %v", err)
	}
	if !containsStr(got.Partial, "contributor_stats") {
		t.Errorf("Partial = %v; want to contain %q", got.Partial, "contributor_stats")
	}
	// TopContributorRecentShare and ContributorCount stay zero.
	if got.TopContributorRecentShare != 0 {
		t.Errorf("TopContributorRecentShare = %.2f; want 0", got.TopContributorRecentShare)
	}
}

// ---------------------------------------------------------------------------
// Releases: exclude draft + prerelease; signed asset detection
// ---------------------------------------------------------------------------

func TestCollect_ReleasesFilteredAndSigned(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	t1 := now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// 1 real release with .sigstore asset; 1 prerelease; 1 draft.
	relJSON := fmt.Sprintf(`[
		{"tag_name":"v1.0.0","prerelease":false,"draft":false,
		 "published_at":%q,
		 "assets":[{"name":"app.tar.gz.sigstore"}]},
		{"tag_name":"v1.0.0-rc1","prerelease":true,"draft":false,
		 "published_at":%q,"assets":[]},
		{"tag_name":"v0.9.0-draft","prerelease":false,"draft":true,
		 "published_at":%q,"assets":[]}
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
			_, _ = fmt.Fprint(w, relJSON)
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
		t.Fatalf("Collect: %v", err)
	}
	if got.ReleaseCount != 1 {
		t.Errorf("ReleaseCount = %d; want 1 (draft and prerelease excluded)", got.ReleaseCount)
	}
	if !got.HasSignedReleaseAssets {
		t.Error("HasSignedReleaseAssets should be true (.sigstore asset)")
	}
	if got.DaysSinceLastRelease != 10 {
		t.Errorf("DaysSinceLastRelease = %d; want 10", got.DaysSinceLastRelease)
	}
}
