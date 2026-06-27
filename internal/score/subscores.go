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
//
// B6: MergedPRs and ClosedUnmergedPRs are NOT all-time totals despite their
// names. The collector fetches the most recently-updated 100 closed pull
// requests (one API page, sorted updated-desc, page cap 100); see
// metrics.Collect. "No closed PRs → 50" therefore means "no recently-updated
// closed PRs in the page", not "no PRs ever closed".
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

// --- Community / Contributable additions (B3) --------------------------------

// PR-responsiveness thresholds for open-PR ghosting. Exported as constants so
// test cases can reference them symbolically rather than embedding magic numbers.
const (
	// prResponsivenessAgeLo is the median open-PR age (days) at or below which
	// the freshness component scores 100.
	prResponsivenessAgeLo = 14.0
	// prResponsivenessAgeHi is the median open-PR age (days) at or above which
	// the freshness component scores 0.
	prResponsivenessAgeHi = 180.0
	// prResponsivenessMaxStale is the number of stale-newcomer open PRs at or
	// above which the stale-penalty component scores 0.
	prResponsivenessMaxStale = 5.0
)

// prResponsiveness scores open-PR ghosting: a maintainer who lets open PRs
// (especially newcomer PRs) sit for months is hard to contribute to,
// regardless of their merge rate on already-closed PRs.
//
// When OpenPRCount == 0 the signal is absent (no open PRs could mean healthy
// velocity OR no incoming PRs at all); defaults to neutral 50.
//
// Otherwise the score blends two components:
//
//	freshness    = linearDown(MedianOpenPRAgeDays, ageLo=14d, ageHi=180d)
//	staleScore   = clamp((maxStale − StaleNewcomerOpenPRs) / maxStale × 100, 0, 100)
//	value        = 0.6 × freshness + 0.4 × staleScore
//
// Rationale for 0.6/0.4 bias: median age captures the whole PR queue; stale
// newcomer count is the most contributor-relevant signal but is a subset.
func prResponsiveness(raw RawMetrics) SubScore {
	const formula = "0 open PRs → 50; 0.6·freshness(median age) + 0.4·stale-penalty"
	if raw.OpenPRCount == 0 {
		return SubScore{
			Key:     "pr_responsiveness",
			Label:   "PR responsiveness",
			Value:   50,
			Formula: formula,
			Raw:     "no open PRs",
		}
	}
	freshness := linearDown(raw.MedianOpenPRAgeDays, prResponsivenessAgeLo, prResponsivenessAgeHi)
	staleScore := clamp((prResponsivenessMaxStale-float64(raw.StaleNewcomerOpenPRs))/prResponsivenessMaxStale*100, 0, 100)
	value := 0.6*freshness + 0.4*staleScore
	return SubScore{
		Key:     "pr_responsiveness",
		Label:   "PR responsiveness",
		Value:   value,
		Formula: formula,
		Raw: fmt.Sprintf("median %.0fd open, %d stale newcomer PRs",
			raw.MedianOpenPRAgeDays, raw.StaleNewcomerOpenPRs),
	}
}

// newcomerSignalsPerIssue is the points-per-labelled-issue used by
// newcomerSignals. Ten labelled issues earns a full 100; one earns 10. The
// signal saturates at 100 so a large issue backlog cannot dominate the score.
const newcomerSignalsPerIssue = 10.0

// newcomerSignals scores the presence of curated entry points: open issues
// labelled "good first issue" or "help wanted". A project that curates
// beginner tasks is signalling contributor intent, which is a positive
// Contributable signal.
//
// Formula: min(100, (GoodFirstIssues + HelpWantedIssues) × 10)
//
// This is intentionally modest (weight 0.05 in Community) so it acts as a
// bonus for welcoming projects rather than a way to override the
// closed_to_strangers gate or the newcomer_merge_rate signal. The
// closed_to_strangers gate still governs the Contributable question as a cap:
// a project can label many issues but if newcomers' PRs are never merged, the
// gate fires and depresses the Contributable score regardless.
func newcomerSignals(raw RawMetrics) SubScore {
	const formula = "min(100, (good-first-issue + help-wanted) × 10)"
	total := raw.GoodFirstIssues + raw.HelpWantedIssues
	value := clamp(float64(total)*newcomerSignalsPerIssue, 0, 100)
	rawDesc := fmt.Sprintf("%d labelled issues (%d gfi, %d hw)",
		total, raw.GoodFirstIssues, raw.HelpWantedIssues)
	return SubScore{
		Key:     "newcomer_signals",
		Label:   "Newcomer signals",
		Value:   value,
		Formula: formula,
		Raw:     rawDesc,
	}
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
