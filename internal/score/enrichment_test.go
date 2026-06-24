package score

import (
	"slices"
	"testing"
)

// TestSubScoreFormula asserts every sub-score carries the documented formula
// string (matching docs/SPEC.md §Sub-scores). One case per sub-score.
func TestSubScoreFormula(t *testing.T) {
	r := Evaluate(healthyRaw())

	want := map[string]string{
		"commit_frequency":     "min(100, median12/15 × 100)",
		"commit_recency":       "max(0, 100 − days/365 × 100)",
		"release_cadence":      "0 releases → 40; else linear 90→730d",
		"issue_close_ratio":    "closed / (closed+open), 90d cohort",
		"pr_backlog":           "merged / (merged+open), 90d cohort",
		"bus_factor":           "0.6·concentration + 0.4·pool; no data → 50",
		"issue_responsiveness": "≤24h→100; ≤168h→100..60; ≤720h→60..0; else 0",
		"pr_acceptance":        "merged / (merged+rejected) × 100",
		"newcomer_merge_rate":  "merged / (merged+rejected) × 100",
		"governance_docs":      "README·.4 + CONTRIB·.35 + CoC·.25",
		"license":              "recognized SPDX → 100; else 0",
		"ci_present":           "CI present → 100; else 0",
		"signed_releases":      "signed → 100; no releases → 40; else 0",
		"security_policy":      "policy present → 100; else 0",
		"workflow_safety":      "pull_request_target → 30; unfetched → 70; else 100",
	}

	for key, wantFormula := range want {
		sub := findSub(t, r, key)
		if sub.Formula != wantFormula {
			t.Errorf("%s Formula = %q, want %q", key, sub.Formula, wantFormula)
		}
	}

	// Every sub-score in the report must carry a non-empty formula.
	for _, c := range r.Categories {
		for _, s := range c.Subs {
			if s.Formula == "" {
				t.Errorf("sub-score %q has empty Formula", s.Key)
			}
		}
	}
}

// TestSubScoreGateLinks asserts each sub-score declares the gates whose trigger
// condition references it, and that non-referenced sub-scores carry no links.
func TestSubScoreGateLinks(t *testing.T) {
	r := Evaluate(healthyRaw())

	want := map[string][]string{
		"pr_acceptance":       {"closed_to_strangers"},
		"newcomer_merge_rate": {"closed_to_strangers"},
		"commit_recency":      {"stale_or_archived"},
		"issue_close_ratio":   {"stale_or_archived"},
		"release_cadence":     {"stale_or_archived"},
		"workflow_safety":     {"integrity_risk"},
		"signed_releases":     {"integrity_risk"},
	}

	// Real gate keys, for validating every link points at an actual gate.
	realGateKeys := map[string]bool{
		"bus_factor": true, "closed_to_strangers": true, "stale_or_archived": true,
		"integrity_risk": true, "vanity_stars": true,
	}

	for _, c := range r.Categories {
		for _, s := range c.Subs {
			wantLinks := want[s.Key] // nil when not referenced
			if !slices.Equal(s.Gates, wantLinks) {
				t.Errorf("%s Gates = %v, want %v", s.Key, s.Gates, wantLinks)
			}
			for _, gk := range s.Gates {
				if !realGateKeys[gk] {
					t.Errorf("%s links to unknown gate %q", s.Key, gk)
				}
			}
		}
	}
}

// TestEveryGateHasHowToClear asserts every triggered gate carries advisory
// guidance, mirroring TestEveryGateHasDetail.
func TestEveryGateHasHowToClear(t *testing.T) {
	raw := healthyRaw()
	raw.Archived = true
	raw.TopContributorRecentShare = 0.9
	raw.ContributorCount = 1
	raw.Stars = 50000
	raw.Watchers = 1
	for _, g := range Evaluate(raw).Gates {
		if g.HowToClear == "" {
			t.Errorf("gate %q missing HowToClear", g.Key)
		}
	}
}

// TestGateHowToClearContent spot-checks the advisory text per gate constructor.
func TestGateHowToClearContent(t *testing.T) {
	cases := []struct {
		name    string
		raw     func() RawMetrics
		gateKey string
		want    string
	}{
		{
			name: "bus_factor",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.TopContributorRecentShare = 0.9
				r.ContributorCount = 1
				return r
			},
			gateKey: "bus_factor",
			want:    "Distribute commits beyond the top author and grow the contributor base.",
		},
		{
			name: "closed_to_strangers",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.MergedPRs = 90
				r.ClosedUnmergedPRs = 10
				r.NewcomerPRsMerged = 1
				r.NewcomerPRsClosedUnmerged = 19
				return r
			},
			gateKey: "closed_to_strangers",
			want:    "Merge PRs from first-time and non-member contributors.",
		},
		{
			name: "stale",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.DaysSinceLastPush = 400
				r.RepoAgeDays = 100
				return r
			},
			gateKey: "stale_or_archived",
			want:    "Resume commits or cut a release.",
		},
		{
			name: "archived",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.Archived = true
				return r
			},
			gateKey: "stale_or_archived",
			want:    "Archived in place; informational only.",
		},
		{
			name: "stale_mature",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.DaysSinceLastPush = 400
				r.RepoAgeDays = 2000
				r.RecentIssuesClosed = 90 // cohort close ratio 90/(90+10) = 90 >= 70
				r.RecentIssuesOpen = 10
				r.ReleaseCount = 5
				r.Archived = false
				return r
			},
			gateKey: "stale_or_archived",
			want:    "Informational: established project, low recent activity.",
		},
		{
			name: "integrity_risk",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.UsesPullRequestTarget = true
				r.WorkflowsFetched = true
				r.HasSignedReleaseAssets = false
				return r
			},
			gateKey: "integrity_risk",
			want:    "Sign release assets and drop pull_request_target workflows.",
		},
		{
			name: "vanity_stars",
			raw: func() RawMetrics {
				r := healthyRaw()
				r.Stars = 50000
				r.Watchers = 100
				return r
			},
			gateKey: "vanity_stars",
			want:    "Informational: stars are high relative to watchers.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, ok := gateByKey(Evaluate(tc.raw()), tc.gateKey)
			if !ok {
				t.Fatalf("%s gate did not trigger", tc.gateKey)
			}
			if g.HowToClear != tc.want {
				t.Errorf("HowToClear = %q, want %q", g.HowToClear, tc.want)
			}
		})
	}
}
