package score

import (
	"fmt"
	"sort"
)

const commitFrequencyFormula = "min(100, commits-per-week/15 × 100)"

func commitFrequency(raw RawMetrics) SubScore {
	var perWeek float64
	var rawDesc string
	switch {
	case len(raw.CommitsLast52Weeks) > 0:
		perWeek = medianLast(raw.CommitsLast52Weeks, 12)
		rawDesc = fmt.Sprintf("%.1f commits/wk (median, 12wk)", perWeek)
	case raw.HasCommitFallback:
		perWeek = raw.CommitsPerWeekFallback
		rawDesc = fmt.Sprintf("~%.1f commits/wk (12wk avg)", perWeek)
	default:
		return SubScore{
			Key:     "commit_frequency",
			Label:   "Commit frequency",
			Value:   50,
			Formula: commitFrequencyFormula,
			Raw:     "commit stats unavailable",
		}
	}
	return SubScore{
		Key:     "commit_frequency",
		Label:   "Commit frequency",
		Value:   clamp(perWeek/15*100, 0, 100),
		Formula: commitFrequencyFormula,
		Raw:     rawDesc,
	}
}

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

func issueCloseRatio(raw RawMetrics) SubScore {
	total := raw.RecentIssuesClosed + raw.RecentIssuesOpen
	return ratioScore(raw.RecentIssuesClosed, total, "issue_close_ratio", "Issue close ratio",
		"closed / (closed+open), 90d cohort",
		fmt.Sprintf("%d/%d issues closed (90d)", raw.RecentIssuesClosed, total))
}

func prBacklog(raw RawMetrics) SubScore {
	total := raw.RecentPRsMerged + raw.RecentPRsOpen
	return ratioScore(raw.RecentPRsMerged, total, "pr_backlog", "PR backlog",
		"merged / (merged+open), 90d cohort",
		fmt.Sprintf("%d merged / %d open (90d)", raw.RecentPRsMerged, raw.RecentPRsOpen))
}

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
		value = 100 - (h-24)/(168-24)*40
		rawDesc = fmt.Sprintf("%.0fh to first response", h)
	case h <= 720:
		value = 60 - (h-168)/(720-168)*60
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

func prAcceptance(raw RawMetrics) SubScore {
	total := raw.MergedPRs + raw.ClosedUnmergedPRs
	return ratioScore(raw.MergedPRs, total, "pr_acceptance", "PR acceptance",
		"merged / (merged+rejected) × 100",
		fmt.Sprintf("%d merged / %d rejected", raw.MergedPRs, raw.ClosedUnmergedPRs))
}

func newcomerMergeRate(raw RawMetrics) SubScore {
	total := raw.NewcomerPRsMerged + raw.NewcomerPRsClosedUnmerged
	return ratioScore(raw.NewcomerPRsMerged, total, "newcomer_merge_rate", "Newcomer merge rate",
		"merged / (merged+rejected) × 100",
		fmt.Sprintf("%d/%d newcomer PRs merged", raw.NewcomerPRsMerged, total))
}

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

func ciPresent(raw RawMetrics) SubScore {
	return boolScore("ci_present", "CI present", raw.HasCI, "CI active", "no CI",
		"CI present → 100; else 0")
}

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
	return SubScore{
		Key: "signed_releases", Label: "Signed releases", Value: value,
		Formula: "signed → 100; no releases → 40; else 0", Raw: desc,
	}
}

func securityPolicy(raw RawMetrics) SubScore {
	return boolScore("security_policy", "Security policy", raw.HasSecurityPolicy,
		"policy present", "no policy", "policy present → 100; else 0")
}

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
	return SubScore{
		Key: "workflow_safety", Label: "Workflow safety", Value: value,
		Formula: "pull_request_target → 30; unfetched → 70; else 100", Raw: desc,
	}
}

const (
	prResponsivenessAgeLo    = 14.0
	prResponsivenessAgeHi    = 180.0
	prResponsivenessMaxStale = 5.0
)

// prResponsiveness blends median open-PR age (0.6) with stale-newcomer count (0.4); age captures the full queue while stale-newcomer count is the contributor-relevant subset.
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

const (
	newcomerSignalsAvailable = 100.0
	newcomerSignalsClaimed   = 60.0
	newcomerSignalsNone      = 50.0
)

func newcomerSignals(raw RawMetrics) SubScore {
	const formula = "available→100; present(claimed)→60; none/unknown→50"
	base := SubScore{Key: "newcomer_signals", Label: "Newcomer signals", Formula: formula}
	switch {
	case !raw.NewcomerLabelsAvailable:
		base.Value, base.Raw = newcomerSignalsNone, "label data unavailable"
	case raw.NewcomerLabeledAvailable > 0:
		base.Value = newcomerSignalsAvailable
		base.Raw = fmt.Sprintf("%d available beginner issues (%d labelled open)",
			raw.NewcomerLabeledAvailable, raw.NewcomerLabeledOpen)
	case raw.NewcomerLabeledOpen > 0:
		base.Value = newcomerSignalsClaimed
		base.Raw = fmt.Sprintf("%d beginner issues, all assigned", raw.NewcomerLabeledOpen)
	default:
		base.Value, base.Raw = newcomerSignalsNone, "no beginner-labelled issues"
	}
	return base
}

func ratioScore(numerator, total int, key, label, formula, raw string) SubScore {
	value := 50.0
	if total > 0 {
		value = float64(numerator) / float64(total) * 100
	}
	return SubScore{Key: key, Label: label, Value: value, Formula: formula, Raw: raw}
}

func boolScore(key, label string, ok bool, yes, no, formula string) SubScore {
	value := 0.0
	desc := no
	if ok {
		value = 100
		desc = yes
	}
	return SubScore{Key: key, Label: label, Value: value, Formula: formula, Raw: desc}
}

func linearDown(x, lo, hi float64) float64 {
	if x <= lo {
		return 100
	}
	if x >= hi {
		return 0
	}
	return (hi - x) / (hi - lo) * 100
}

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
