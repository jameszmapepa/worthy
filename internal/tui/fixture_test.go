package tui

import "github.com/jameszmapepa/repo-health/internal/score"

// fixedReport returns a deterministic Report used across the view and model
// tests. It exercises all three categories, a range of bar colors, and one
// gate of each severity so the renderers' branches are covered.
func fixedReport() score.Report {
	cap75 := 75.0
	return score.Report{
		Categories: []score.CategoryScore{
			{
				Key: score.CategoryActivity, Label: "Activity", Value: 82.5, Weight: 0.40,
				Subs: []score.SubScore{
					{Key: "commit_frequency", Label: "Commit frequency", Value: 90, Raw: "13.5 commits/wk", Formula: "min(100, median12/15 × 100)", Weight: 0.2},
					{Key: "commit_recency", Label: "Commit recency", Value: 55, Raw: "164d since last push", Formula: "max(0, 100 − days/365 × 100)", Weight: 0.2, Gates: []string{"stale_or_archived"}},
					{Key: "release_cadence", Label: "Release cadence", Value: 30, Raw: "300d since last release", Formula: "0 releases → 40; else linear 90→730d", Weight: 0.2, Gates: []string{"stale_or_archived"}},
				},
			},
			{
				Key: score.CategoryCommunity, Label: "Community", Value: 64.0, Weight: 0.30,
				Subs: []score.SubScore{
					{Key: "issue_responsiveness", Label: "Issue responsiveness", Value: 64, Raw: "96h to first response", Weight: 0.2},
					{Key: "license", Label: "License", Value: 100, Raw: "MIT", Weight: 0.2},
				},
			},
			{
				Key: score.CategorySecurity, Label: "Security", Value: 50.0, Weight: 0.30,
				Subs: []score.SubScore{
					{Key: "ci_present", Label: "CI present", Value: 100, Raw: "CI active", Weight: 0.25},
					{Key: "workflow_safety", Label: "Workflow safety", Value: 30, Raw: "uses pull_request_target", Weight: 0.25},
				},
			},
		},
		Composite:         68.2,
		AdjustedComposite: 68.2,
		Grade:             "C",
		Verdict:           "In fair health (grade C): strongest on license, weakest on workflow safety; flagged closed to newcomers.",
		Gates: []score.Gate{
			{Key: "closed_to_strangers", Severity: score.SeverityWarn, Title: "Closed to newcomers", Detail: "Newcomers' PRs are rarely merged.", HowToClear: "Merge PRs from first-time and non-member contributors.", CapTo: &cap75},
			{Key: "vanity_stars", Severity: score.SeverityInfo, Title: "Stars outpace engagement", Detail: "High stars relative to watchers.", HowToClear: "Informational: stars are high relative to watchers."},
		},
	}
}

// healthyFixedReport returns a deterministic Report with NO gates, for the
// Explain view's healthy-repo empty state.
func healthyFixedReport() score.Report {
	return score.Report{
		Categories: []score.CategoryScore{
			{
				Key: score.CategoryActivity, Label: "Activity", Value: 92.0, Weight: 0.40,
				Subs: []score.SubScore{
					{Key: "commit_frequency", Label: "Commit frequency", Value: 95, Raw: "14 commits/wk", Weight: 0.5},
					{Key: "commit_recency", Label: "Commit recency", Value: 89, Raw: "5d since last push", Weight: 0.5},
				},
			},
			{
				Key: score.CategorySecurity, Label: "Security", Value: 80.0, Weight: 0.30,
				Subs: []score.SubScore{
					{Key: "ci_present", Label: "CI present", Value: 100, Raw: "CI active", Weight: 0.5},
					{Key: "workflow_safety", Label: "Workflow safety", Value: 60, Raw: "workflows not inspected", Weight: 0.5},
				},
			},
		},
		Composite:         88.0,
		AdjustedComposite: 88.0,
		Grade:             "A",
		Verdict:           "In excellent health (grade A): strongest on CI present, weakest on workflow safety.",
		Gates:             nil,
	}
}

// fixedRaw returns deterministic RawMetrics for the gauges/sparkline view.
func fixedRaw() score.RawMetrics {
	return score.RawMetrics{
		CommitsLast52Weeks: []int{1, 4, 2, 8, 5, 9, 3, 7, 6, 10, 4, 8},
		Stars:              4200,
		Forks:              310,
		Watchers:           120,
	}
}

// realReport runs the real scorer over a representative metrics snapshot, for
// tests that need an end-to-end Report (verdict text, gate evaluation) rather
// than the hand-built fixedReport.
func realReport() score.Report {
	return score.Evaluate(score.RawMetrics{
		CommitsLast52Weeks:            []int{5, 6, 7, 8, 6, 5, 9, 7, 6, 8, 5, 7},
		DaysSinceLastPush:             10,
		RepoAgeDays:                   800,
		OpenIssues:                    20,
		ClosedIssues:                  180,
		OpenPRs:                       5,
		MergedPRs:                     60,
		ClosedUnmergedPRs:             8,
		MedianIssueFirstResponseHours: 30,
		NewcomerPRsMerged:             5,
		NewcomerPRsClosedUnmerged:     3,
		TopContributorRecentShare:     0.5,
		ContributorCount:              8,
		ReleaseCount:                  6,
		DaysSinceLastRelease:          120,
		HasReadme:                     true,
		HasContributing:               true,
		HasLicense:                    true,
		LicenseSPDX:                   "Apache-2.0",
		HasCI:                         true,
		HasSecurityPolicy:             false,
		WorkflowsFetched:              true,
		Stars:                         3000,
		Watchers:                      400,
	})
}
