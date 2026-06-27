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
// Happy path — end-to-end Collect with a fully-stubbed server
// ---------------------------------------------------------------------------

func TestCollect_HappyPath(t *testing.T) {
	// Fixed "now" so all time computations are deterministic.
	// pushed_at = 2026-05-22 → 31 days before now (2026-06-22).
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	pushedAt, _ := time.Parse(time.RFC3339, "2026-05-22T10:00:00Z")
	expectedDaysSincePush := int(now.Sub(pushedAt).Hours() / 24) // ~30

	createdAt, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	expectedRepoAge := int(now.Sub(createdAt).Hours() / 24) // ~2364

	releasesBody := buildReleasesJSON(now)
	closedPRsBody := buildClosedPRsJSON(now)
	recentIssuesBody := buildRecentIssuesJSON(now)

	// Issue 11 (eve, created 10h before now) → first comment 2h later = 2h TTFR.
	// Issue 12 (frank, no comments) → no qualifying response.
	issueCreated := now.Add(-10 * time.Hour)
	issue11Comments := buildIssueCommentsJSON(now, issueCreated, false) // 2h response, human
	issue12Comments := `[]`                                             // no comments

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Core repo
		case path == "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)

		// Community profile
		case path == "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)

		// Contributor stats
		case path == "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildContributorStatsJSON())

		// Commit activity
		case path == "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildCommitActivityJSON())

		// Releases
		case path == "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releasesBody)

		// Workflows list
		case path == "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON)

		// Workflow file contents (raw)
		case path == "/repos/acme/widget/contents/.github/workflows/ci.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, ciYAML)
		case path == "/repos/acme/widget/contents/.github/workflows/release.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releaseYAML)

		// RecentPulls — closed page for newcomer analysis (sort=updated)
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "closed" && r.URL.Query().Get("sort") == "updated":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsBody)

		// RecentPulls — open page for open-PR ghosting metrics (A2)
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "open" && r.URL.Query().Get("sort") == "updated":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`) // no open PRs in the happy-path fixture

		// RecentPullsByCreation — 90-day cohort (state=all, sort=created)
		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "all" && r.URL.Query().Get("sort") == "created":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsBody) // reuse same fixture; PR1 merged, PR2 closed-unmerged (excluded), PR3 merged, PR4 merged

		// RecentIssues — all, for TTFR sampling and issue cohort
		case path == "/repos/acme/widget/issues" && r.URL.Query().Get("state") == "all" && r.URL.Query().Get("per_page") == "100":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, recentIssuesBody)

		// IssueComments for issue 11 and 12 (10 is a PR, filtered out)
		case path == "/repos/acme/widget/issues/11/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue11Comments)
		case path == "/repos/acme/widget/issues/12/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue12Comments)

		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":"unexpected path %s query %s"}`, path, r.URL.RawQuery)
		}
	}))
	defer srv.Close()

	c := client(srv)
	got, err := Collect(context.Background(), c, "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// --- Repository core ---
	if got.Stars != 1500 {
		t.Errorf("Stars = %d; want 1500", got.Stars)
	}
	if got.Forks != 200 {
		t.Errorf("Forks = %d; want 200", got.Forks)
	}
	if got.Watchers != 80 {
		t.Errorf("Watchers = %d; want 80", got.Watchers)
	}
	if got.Archived {
		t.Error("Archived should be false")
	}
	if got.Disabled {
		t.Error("Disabled should be false")
	}
	if got.LicenseSPDX != "MIT" {
		t.Errorf("LicenseSPDX = %q; want %q", got.LicenseSPDX, "MIT")
	}
	if got.DaysSinceLastPush != expectedDaysSincePush {
		t.Errorf("DaysSinceLastPush = %d; want ~%d", got.DaysSinceLastPush, expectedDaysSincePush)
	}
	if got.RepoAgeDays != expectedRepoAge {
		t.Errorf("RepoAgeDays = %d; want ~%d", got.RepoAgeDays, expectedRepoAge)
	}

	// --- Governance ---
	if got.HealthPercentage != 72 {
		t.Errorf("HealthPercentage = %d; want 72", got.HealthPercentage)
	}
	if !got.HasReadme {
		t.Error("HasReadme should be true")
	}
	if got.HasContributing {
		t.Error("HasContributing should be false")
	}
	if got.HasCodeOfConduct {
		t.Error("HasCodeOfConduct should be false")
	}
	if !got.HasSecurityPolicy {
		t.Error("HasSecurityPolicy should be true")
	}

	// --- Contributor stats ---
	// alice: 10*12=120, bob: 2*12=24 → total=144, alice share=120/144≈0.833
	wantAliceShare := 120.0 / 144.0
	if got.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d; want 2", got.ContributorCount)
	}
	if got.TopContributorRecentShare < wantAliceShare-0.01 || got.TopContributorRecentShare > wantAliceShare+0.01 {
		t.Errorf("TopContributorRecentShare = %.4f; want ≈%.4f", got.TopContributorRecentShare, wantAliceShare)
	}

	// --- CommitsLast52Weeks ---
	if len(got.CommitsLast52Weeks) != 52 {
		t.Errorf("len(CommitsLast52Weeks) = %d; want 52", len(got.CommitsLast52Weeks))
	}

	// --- Releases ---
	// Only 2 non-draft, non-pre-release; most recent is 45 days ago.
	if got.ReleaseCount != 2 {
		t.Errorf("ReleaseCount = %d; want 2", got.ReleaseCount)
	}
	if got.DaysSinceLastRelease != 45 {
		t.Errorf("DaysSinceLastRelease = %d; want 45", got.DaysSinceLastRelease)
	}
	if !got.HasSignedReleaseAssets {
		t.Error("HasSignedReleaseAssets should be true (.asc asset present)")
	}

	// --- CI ---
	if !got.HasCI {
		t.Error("HasCI should be true (one active workflow)")
	}

	// --- Workflow safety ---
	if !got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be true")
	}
	if !got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be true (release.yml uses it)")
	}

	// --- PR outcome split (all-time, from closed PRs page) ---
	// Closed PRs page: PR1 merged, PR2 closed-unmerged, PR3 merged, PR4 merged.
	// MergedPRs = 3; ClosedUnmergedPRs = 1.
	if got.MergedPRs != 3 {
		t.Errorf("MergedPRs = %d; want 3", got.MergedPRs)
	}
	if got.ClosedUnmergedPRs != 1 {
		t.Errorf("ClosedUnmergedPRs = %d; want 1", got.ClosedUnmergedPRs)
	}

	// --- 90-day issue creation cohort ---
	// recentIssuesBody: issue 10 is a PR (filtered), issue 11+12 are open within 90d.
	// RecentIssuesClosed=0, RecentIssuesOpen=2.
	if got.RecentIssuesClosed != 0 {
		t.Errorf("RecentIssuesClosed = %d; want 0", got.RecentIssuesClosed)
	}
	if got.RecentIssuesOpen != 2 {
		t.Errorf("RecentIssuesOpen = %d; want 2", got.RecentIssuesOpen)
	}

	// --- 90-day PR creation cohort ---
	// closedPRsBody reused: PR1 merged (30d ago, in window), PR2 closed-unmerged (excluded),
	// PR3 merged (30d ago, in window), PR4 merged (200d ago, outside window).
	// RecentPRsMerged=2, RecentPRsOpen=0.
	if got.RecentPRsMerged != 2 {
		t.Errorf("RecentPRsMerged = %d; want 2", got.RecentPRsMerged)
	}
	if got.RecentPRsOpen != 0 {
		t.Errorf("RecentPRsOpen = %d; want 0", got.RecentPRsOpen)
	}

	// --- Newcomer PRs ---
	// PR1: FIRST_TIME_CONTRIBUTOR, merged by maintainer (not self), within 90d → NewcomerPRsMerged
	// PR2: NONE, closed-unmerged, within 90d → NewcomerPRsClosedUnmerged
	// PR3: CONTRIBUTOR, but self-merged → excluded newcomer counts
	// PR4: MEMBER → not newcomer
	if got.NewcomerPRsMerged != 1 {
		t.Errorf("NewcomerPRsMerged = %d; want 1", got.NewcomerPRsMerged)
	}
	if got.NewcomerPRsClosedUnmerged != 1 {
		t.Errorf("NewcomerPRsClosedUnmerged = %d; want 1", got.NewcomerPRsClosedUnmerged)
	}

	// --- TTFR ---
	// Issue 10 PR → filtered. Issue 11: eve, first response by maintainer 2h later.
	// Issue 12: frank, no comments → no response. Median of [2.0] = 2.0h.
	if got.MedianIssueFirstResponseHours < 1.9 || got.MedianIssueFirstResponseHours > 2.1 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≈2.0", got.MedianIssueFirstResponseHours)
	}

	// --- A4: Fork flag ---
	if got.Fork {
		t.Error("Fork should be false (repoJSON has fork:false)")
	}

	// --- A2: open-PR ghosting — no open PRs in fixture, all zeros ---
	if got.OpenPRCount != 0 {
		t.Errorf("OpenPRCount = %d; want 0 (no open PRs in fixture)", got.OpenPRCount)
	}
	if got.MedianOpenPRAgeDays != 0 {
		t.Errorf("MedianOpenPRAgeDays = %.2f; want 0", got.MedianOpenPRAgeDays)
	}
	if got.StaleNewcomerOpenPRs != 0 {
		t.Errorf("StaleNewcomerOpenPRs = %d; want 0", got.StaleNewcomerOpenPRs)
	}

	// --- A3: label counts — recentIssuesBody has no labels, so both zero ---
	if got.GoodFirstIssues != 0 {
		t.Errorf("GoodFirstIssues = %d; want 0 (fixture issues carry no labels)", got.GoodFirstIssues)
	}
	if got.HelpWantedIssues != 0 {
		t.Errorf("HelpWantedIssues = %d; want 0 (fixture issues carry no labels)", got.HelpWantedIssues)
	}

	// No partial metrics in happy path.
	if len(got.Partial) != 0 {
		t.Errorf("Partial = %v; want empty", got.Partial)
	}
}
