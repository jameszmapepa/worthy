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

func TestCollect_HappyPath(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	pushedAt, _ := time.Parse(time.RFC3339, "2026-05-22T10:00:00Z")
	expectedDaysSincePush := int(now.Sub(pushedAt).Hours() / 24)

	createdAt, _ := time.Parse(time.RFC3339, "2020-01-01T00:00:00Z")
	expectedRepoAge := int(now.Sub(createdAt).Hours() / 24)

	releasesBody := buildReleasesJSON(now)
	closedPRsBody := buildClosedPRsJSON(now)
	recentIssuesBody := buildRecentIssuesJSON(now)

	issueCreated := now.Add(-10 * time.Hour)
	issue11Comments := buildIssueCommentsJSON(now, issueCreated, false)
	issue12Comments := `[]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {

		case path == "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, repoJSON)

		case path == "/repos/acme/widget/community/profile":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, communityJSON)

		case path == "/repos/acme/widget/stats/contributors":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildContributorStatsJSON())

		case path == "/repos/acme/widget/stats/commit_activity":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, buildCommitActivityJSON())

		case path == "/repos/acme/widget/releases":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releasesBody)

		case path == "/repos/acme/widget/actions/workflows":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, workflowsJSON)

		case path == "/repos/acme/widget/contents/.github/workflows/ci.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, ciYAML)
		case path == "/repos/acme/widget/contents/.github/workflows/release.yml":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, releaseYAML)

		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "closed" && r.URL.Query().Get("sort") == "updated":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsBody)

		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "open" && r.URL.Query().Get("sort") == "updated":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)

		case path == "/repos/acme/widget/pulls" && r.URL.Query().Get("state") == "all" && r.URL.Query().Get("sort") == "created":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, closedPRsBody)

		case path == "/repos/acme/widget/issues" && r.URL.Query().Get("state") == "all" && r.URL.Query().Get("per_page") == "100":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, recentIssuesBody)

		case path == "/repos/acme/widget/issues/11/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue11Comments)
		case path == "/repos/acme/widget/issues/12/comments":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, issue12Comments)

		case path == "/search/issues":
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Query().Get("q"), "no:assignee") {
				_, _ = fmt.Fprint(w, `{"total_count":1}`)
			} else {
				_, _ = fmt.Fprint(w, `{"total_count":3}`)
			}

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

	wantAliceShare := 120.0 / 144.0
	if got.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d; want 2", got.ContributorCount)
	}
	if got.TopContributorRecentShare < wantAliceShare-0.01 || got.TopContributorRecentShare > wantAliceShare+0.01 {
		t.Errorf("TopContributorRecentShare = %.4f; want ≈%.4f", got.TopContributorRecentShare, wantAliceShare)
	}

	if len(got.CommitsLast52Weeks) != 52 {
		t.Errorf("len(CommitsLast52Weeks) = %d; want 52", len(got.CommitsLast52Weeks))
	}

	if got.ReleaseCount != 2 {
		t.Errorf("ReleaseCount = %d; want 2", got.ReleaseCount)
	}
	if got.DaysSinceLastRelease != 45 {
		t.Errorf("DaysSinceLastRelease = %d; want 45", got.DaysSinceLastRelease)
	}
	if !got.HasSignedReleaseAssets {
		t.Error("HasSignedReleaseAssets should be true (.asc asset present)")
	}

	if !got.HasCI {
		t.Error("HasCI should be true (one active workflow)")
	}

	if !got.WorkflowsFetched {
		t.Error("WorkflowsFetched should be true")
	}
	if !got.UsesPullRequestTarget {
		t.Error("UsesPullRequestTarget should be true (release.yml uses it)")
	}

	if got.MergedPRs != 3 {
		t.Errorf("MergedPRs = %d; want 3", got.MergedPRs)
	}
	if got.ClosedUnmergedPRs != 1 {
		t.Errorf("ClosedUnmergedPRs = %d; want 1", got.ClosedUnmergedPRs)
	}

	if got.RecentIssuesClosed != 0 {
		t.Errorf("RecentIssuesClosed = %d; want 0", got.RecentIssuesClosed)
	}
	if got.RecentIssuesOpen != 2 {
		t.Errorf("RecentIssuesOpen = %d; want 2", got.RecentIssuesOpen)
	}

	if got.RecentPRsMerged != 2 {
		t.Errorf("RecentPRsMerged = %d; want 2", got.RecentPRsMerged)
	}
	if got.RecentPRsOpen != 0 {
		t.Errorf("RecentPRsOpen = %d; want 0", got.RecentPRsOpen)
	}

	if got.NewcomerPRsMerged != 1 {
		t.Errorf("NewcomerPRsMerged = %d; want 1", got.NewcomerPRsMerged)
	}
	if got.NewcomerPRsClosedUnmerged != 1 {
		t.Errorf("NewcomerPRsClosedUnmerged = %d; want 1", got.NewcomerPRsClosedUnmerged)
	}

	if got.MedianIssueFirstResponseHours < 1.9 || got.MedianIssueFirstResponseHours > 2.1 {
		t.Errorf("MedianIssueFirstResponseHours = %.2f; want ≈2.0", got.MedianIssueFirstResponseHours)
	}

	if got.Fork {
		t.Error("Fork should be false (repoJSON has fork:false)")
	}

	if got.OpenPRCount != 0 {
		t.Errorf("OpenPRCount = %d; want 0 (no open PRs in fixture)", got.OpenPRCount)
	}
	if got.MedianOpenPRAgeDays != 0 {
		t.Errorf("MedianOpenPRAgeDays = %.2f; want 0", got.MedianOpenPRAgeDays)
	}
	if got.StaleNewcomerOpenPRs != 0 {
		t.Errorf("StaleNewcomerOpenPRs = %d; want 0", got.StaleNewcomerOpenPRs)
	}

	if !got.NewcomerLabelsAvailable {
		t.Error("NewcomerLabelsAvailable should be true (search served)")
	}
	if got.NewcomerLabeledOpen != 3 {
		t.Errorf("NewcomerLabeledOpen = %d; want 3", got.NewcomerLabeledOpen)
	}
	if got.NewcomerLabeledAvailable != 1 {
		t.Errorf("NewcomerLabeledAvailable = %d; want 1", got.NewcomerLabeledAvailable)
	}

	if len(got.Partial) != 0 {
		t.Errorf("Partial = %v; want empty", got.Partial)
	}
}
