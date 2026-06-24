package score

import (
	"fmt"
	"sort"
)

// --- Activity ---------------------------------------------------------------

// commitFrequency scores the median weekly commit count over the last 12
// weeks on a saturating curve: 0 -> 0, >=15/wk -> 100, linear between.
//
// An EMPTY series means the commit-activity stats were unavailable (GitHub's
// /stats/commit_activity returns a 202 while recomputing), not that the repo is
// inactive, so it scores a neutral 50 rather than tanking Activity. A series
// that is present but all zero is genuinely inactive and keeps scoring 0.
func commitFrequency(raw RawMetrics) SubScore {
	if len(raw.CommitsLast52Weeks) == 0 {
		return SubScore{
			Key:     "commit_frequency",
			Label:   "Commit frequency",
			Value:   50,
			Formula: "min(100, median12/15 × 100)",
			Raw:     "no commit data",
		}
	}
	median := medianLast(raw.CommitsLast52Weeks, 12)
	value := clamp(median/15*100, 0, 100)
	return SubScore{
		Key:     "commit_frequency",
		Label:   "Commit frequency",
		Value:   value,
		Formula: "min(100, median12/15 × 100)",
		Raw:     fmt.Sprintf("%.1f commits/wk (median, 12wk)", median),
	}
}

// commitRecency scores days since last push: 0d -> 100, >=365d -> 0, linear.
func commitRecency(raw RawMetrics) SubScore {
	value := clamp(100-float64(raw.DaysSinceLastPush)/365*100, 0, 100)
	return SubScore{
		Key:     "commit_recency",
		Label:   "Commit recency",
		Value:   value,
		Formula: "max(0, 100 − days/365 × 100)",
		Raw:     fmt.Sprintf("%dd since last push", raw.DaysSinceLastPush),
	}
}

// releaseCadence scores release recency. No releases -> 40 (neutral-low).
// Otherwise <=90d -> 100, >=730d -> 0, linear between.
func releaseCadence(raw RawMetrics) SubScore {
	var value float64
	var rawDesc string
	if raw.ReleaseCount == 0 {
		value = 40
		rawDesc = "no releases"
	} else {
		value = linearDown(float64(raw.DaysSinceLastRelease), 90, 730)
		rawDesc = fmt.Sprintf("%dd since last release", raw.DaysSinceLastRelease)
	}
	return SubScore{
		Key:     "release_cadence",
		Label:   "Release cadence",
		Value:   value,
		Formula: "0 releases → 40; else linear 90→730d",
		Raw:     rawDesc,
	}
}

// issueCloseRatio scores closed/(closed+open) non-PR issues created in the
// last 90 days. No in-cohort issues -> 50 (neutral/no-data).
func issueCloseRatio(raw RawMetrics) SubScore {
	total := raw.RecentIssuesClosed + raw.RecentIssuesOpen
	return ratioScore(raw.RecentIssuesClosed, total, "issue_close_ratio", "Issue close ratio",
		"closed / (closed+open), 90d cohort",
		fmt.Sprintf("%d/%d issues closed (90d)", raw.RecentIssuesClosed, total))
}

// prBacklog scores merged/(merged+open) PRs created in the last 90 days.
// Closed-unmerged PRs are excluded from both numerator and denominator.
// No in-cohort PRs -> 50 (neutral/no-data).
func prBacklog(raw RawMetrics) SubScore {
	total := raw.RecentPRsMerged + raw.RecentPRsOpen
	return ratioScore(raw.RecentPRsMerged, total, "pr_backlog", "PR backlog",
		"merged / (merged+open), 90d cohort",
		fmt.Sprintf("%d merged / %d open (90d)", raw.RecentPRsMerged, raw.RecentPRsOpen))
}

// busFactor scores how distributed recent authorship is, as a continuous
// companion to the bus_factor GATE (which fires only at the extreme:
// share > 0.80 AND count <= 2). The sub-score gives a graded signal across the
// danger zone the gate misses (e.g. 0.75 share with 3 contributors). It blends
// concentration — how little the top author dominates, (1−share) scaled so a
// top share at/under 0.40 earns full marks — at 0.6 with pool size — the
// contributor count scaled so 5+ earns full marks — at 0.4.
//
// No contributor data (ContributorCount == 0, e.g. commit-activity stats
// unavailable) scores a neutral 50 rather than 0.
//
// ceiling: a proxy over the two metrics we collect (top-1 share + count). The
// exact CHAOSS bus factor (smallest N making 50% of contributions) needs the
// full per-contributor distribution, which we do not fetch.
func busFactor(raw RawMetrics) SubScore {
	const formula = "0.6·concentration + 0.4·pool; no data → 50"
	if raw.ContributorCount == 0 {
		return SubScore{
			Key:     "bus_factor",
			Label:   "Bus factor",
			Value:   50,
			Formula: formula,
			Raw:     "no contributor data",
		}
	}
	concentration := clamp((1-raw.TopContributorRecentShare)/0.6*100, 0, 100)
	pool := clamp(float64(raw.ContributorCount-1)/4*100, 0, 100)
	return SubScore{
		Key:     "bus_factor",
		Label:   "Bus factor",
		Value:   0.6*concentration + 0.4*pool,
		Formula: formula,
		Raw: fmt.Sprintf("top author %.0f%% of commits, %d contributors",
			raw.TopContributorRecentShare*100, raw.ContributorCount),
	}
}

// --- Community / Governance -------------------------------------------------

// issueResponsiveness scores the bot-filtered median time-to-first-response.
// <=24h -> 100; 24-168h -> 100..60; 168-720h -> 60..0; >720h -> 0.
// No issue data (hours <= 0) -> 50.
func issueResponsiveness(raw RawMetrics) SubScore {
	h := raw.MedianIssueFirstResponseHours
	var value float64
	var rawDesc string
	switch {
	case h <= 0:
		value = 50
		rawDesc = "no issue response data"
	case h <= 24:
		value = 100
		rawDesc = fmt.Sprintf("%.0fh to first response", h)
	case h <= 168:
		value = 100 - (h-24)/(168-24)*40 // 100 -> 60
		rawDesc = fmt.Sprintf("%.0fh to first response", h)
	case h <= 720:
		value = 60 - (h-168)/(720-168)*60 // 60 -> 0
		rawDesc = fmt.Sprintf("%.0fh to first response", h)
	default:
		value = 0
		rawDesc = fmt.Sprintf("%.0fh to first response", h)
	}
	return SubScore{
		Key:     "issue_responsiveness",
		Label:   "Issue responsiveness",
		Value:   value,
		Formula: "≤24h→100; ≤168h→100..60; ≤720h→60..0; else 0",
		Raw:     rawDesc,
	}
}

// prAcceptance scores merged/(merged+closedUnmerged) PRs. No closed PRs -> 50.
func prAcceptance(raw RawMetrics) SubScore {
	total := raw.MergedPRs + raw.ClosedUnmergedPRs
	return ratioScore(raw.MergedPRs, total, "pr_acceptance", "PR acceptance",
		"merged / (merged+rejected) × 100",
		fmt.Sprintf("%d merged / %d rejected", raw.MergedPRs, raw.ClosedUnmergedPRs))
}

// newcomerMergeRate scores newcomer merged/(merged+closedUnmerged). No
// newcomer PRs -> 50 (unknown/neutral).
func newcomerMergeRate(raw RawMetrics) SubScore {
	total := raw.NewcomerPRsMerged + raw.NewcomerPRsClosedUnmerged
	return ratioScore(raw.NewcomerPRsMerged, total, "newcomer_merge_rate", "Newcomer merge rate",
		"merged / (merged+rejected) × 100",
		fmt.Sprintf("%d/%d newcomer PRs merged", raw.NewcomerPRsMerged, total))
}

// governanceDocs scores weighted presence of governance docs:
// README .40, CONTRIBUTING .35, CODE_OF_CONDUCT .25, *100. LICENSE is
// deliberately excluded — it is scored by the standalone `license` sub-score, so
// counting it here too would double-count license presence within Community.
func governanceDocs(raw RawMetrics) SubScore {
	var v float64
	if raw.HasReadme {
		v += 0.40
	}
	if raw.HasContributing {
		v += 0.35
	}
	if raw.HasCodeOfConduct {
		v += 0.25
	}
	return SubScore{
		Key:     "governance_docs",
		Label:   "Governance docs",
		Value:   v * 100,
		Formula: "README·.4 + CONTRIB·.35 + CoC·.25",
		Raw:     fmt.Sprintf("%d%% docs present", raw.HealthPercentage),
	}
}

// licenseScore is 100 for a recognized SPDX license, else 0.
func licenseScore(raw RawMetrics) SubScore {
	value := 0.0
	id := raw.LicenseSPDX
	if id != "" && id != "NOASSERTION" {
		value = 100
	}
	desc := id
	if value == 0 {
		desc = "none"
	}
	return SubScore{
		Key:     "license",
		Label:   "License",
		Value:   value,
		Formula: "recognized SPDX → 100; else 0",
		Raw:     desc,
	}
}

// --- Security / Integrity ---------------------------------------------------

// ciPresent is 100 when at least one active workflow exists, else 0.
func ciPresent(raw RawMetrics) SubScore {
	return boolScore("ci_present", "CI present", raw.HasCI, "CI active", "no CI",
		"CI present → 100; else 0")
}

// signedReleases is 100 with signed assets, 40 when there are no releases,
// else 0.
func signedReleases(raw RawMetrics) SubScore {
	var value float64
	var desc string
	switch {
	case raw.HasSignedReleaseAssets:
		value, desc = 100, "signed assets"
	case raw.ReleaseCount == 0:
		value, desc = 40, "no releases"
	default:
		value, desc = 0, "unsigned releases"
	}
	return SubScore{Key: "signed_releases", Label: "Signed releases", Value: value,
		Formula: "signed → 100; no releases → 40; else 0", Raw: desc}
}

// securityPolicy is 100 with a SECURITY policy present, else 0.
func securityPolicy(raw RawMetrics) SubScore {
	return boolScore("security_policy", "Security policy", raw.HasSecurityPolicy,
		"policy present", "no policy", "policy present → 100; else 0")
}

// workflowSafety is 30 when pull_request_target is used, 70 when workflows were
// not fetched (unknown), else 100.
func workflowSafety(raw RawMetrics) SubScore {
	var value float64
	var desc string
	switch {
	case raw.UsesPullRequestTarget:
		value, desc = 30, "uses pull_request_target"
	case !raw.WorkflowsFetched:
		value, desc = 70, "workflows not inspected"
	default:
		value, desc = 100, "no risky triggers"
	}
	return SubScore{Key: "workflow_safety", Label: "Workflow safety", Value: value,
		Formula: "pull_request_target → 30; unfetched → 70; else 100", Raw: desc}
}

// --- helpers ----------------------------------------------------------------

// ratioScore builds a 0..100 sub-score as numerator/total × 100, treating an
// empty denominator (total == 0) as a neutral 50 (no data). The raw metric
// string is supplied by the caller because each ratio describes its inputs
// differently.
func ratioScore(numerator, total int, key, label, formula, raw string) SubScore {
	value := 50.0
	if total > 0 {
		value = float64(numerator) / float64(total) * 100
	}
	return SubScore{Key: key, Label: label, Value: value, Formula: formula, Raw: raw}
}

// boolScore builds a 100/0 sub-score from a boolean.
func boolScore(key, label string, ok bool, yes, no, formula string) SubScore {
	value := 0.0
	desc := no
	if ok {
		value = 100
		desc = yes
	}
	return SubScore{Key: key, Label: label, Value: value, Formula: formula, Raw: desc}
}

// linearDown maps x to 100 at or below lo, 0 at or above hi, linear between.
func linearDown(x, lo, hi float64) float64 {
	if x <= lo {
		return 100
	}
	if x >= hi {
		return 0
	}
	return (hi - x) / (hi - lo) * 100
}

// medianLast returns the median of the last n elements of s (or all of s when
// it has fewer than n). An empty input yields 0.
func medianLast(s []int, n int) float64 {
	if len(s) == 0 {
		return 0
	}
	if len(s) > n {
		s = s[len(s)-n:]
	}
	vals := make([]int, len(s))
	copy(vals, s)
	sort.Ints(vals)
	mid := len(vals) / 2
	if len(vals)%2 == 1 {
		return float64(vals[mid])
	}
	return float64(vals[mid-1]+vals[mid]) / 2
}
