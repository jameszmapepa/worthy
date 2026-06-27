package score

// ConfidenceLevel indicates how much real data backed a scored Report. The
// lower the confidence, the more sub-scores fell back to their documented
// neutral/no-data defaults (a neutral-50 sentinel returned when a GitHub API
// cohort was empty or unavailable), which can make a low-signal repo appear
// misleadingly precise.
type ConfidenceLevel int

const (
	// ConfidenceLow: ≥5 sub-scores fell back to neutral defaults. The score
	// is structurally valid but likely imprecise; the TUI should surface a
	// caveat such as "limited data".
	ConfidenceLow ConfidenceLevel = iota
	// ConfidenceMedium: 2–4 sub-scores fell back to neutral defaults. Some
	// signals are missing but the composite is broadly meaningful.
	ConfidenceMedium
	// ConfidenceHigh: ≤1 sub-score fell back to a neutral default. The
	// scoring had enough real data to be reliable.
	ConfidenceHigh
)

// String implements fmt.Stringer for human-readable display.
func (c ConfidenceLevel) String() string {
	switch c {
	case ConfidenceLow:
		return "Low"
	case ConfidenceMedium:
		return "Medium"
	case ConfidenceHigh:
		return "High"
	default:
		return "Unknown"
	}
}

// neutralDefaultCount counts sub-score inputs that fell back to the
// documented neutral-50 no-data sentinel. Only sub-scores included in the
// category aggregates are counted; signals excluded from the composite (e.g.
// busFactor after B5) are omitted.
//
// Counted signals and their neutral condition:
//   - commit_frequency    : no stats series AND no commits-count fallback
//   - issue_responsiveness: MedianIssueFirstResponseHours ≤ 0
//   - pr_acceptance       : MergedPRs + ClosedUnmergedPRs == 0
//   - newcomer_merge_rate : NewcomerPRsMerged + NewcomerPRsClosedUnmerged == 0
//   - issue_close_ratio   : RecentIssuesClosed + RecentIssuesOpen == 0
//   - pr_backlog          : RecentPRsMerged + RecentPRsOpen == 0
//   - pr_responsiveness   : OpenPRCount == 0
func neutralDefaultCount(raw RawMetrics) int {
	n := 0
	if len(raw.CommitsLast52Weeks) == 0 && !raw.HasCommitFallback {
		n++
	}
	if raw.MedianIssueFirstResponseHours <= 0 {
		n++
	}
	if raw.MergedPRs+raw.ClosedUnmergedPRs == 0 {
		n++
	}
	if raw.NewcomerPRsMerged+raw.NewcomerPRsClosedUnmerged == 0 {
		n++
	}
	if raw.RecentIssuesClosed+raw.RecentIssuesOpen == 0 {
		n++
	}
	if raw.RecentPRsMerged+raw.RecentPRsOpen == 0 {
		n++
	}
	if raw.OpenPRCount == 0 {
		n++
	}
	return n
}

// computeConfidence returns the ConfidenceLevel based on how many of the
// seven tracked sub-score inputs fell back to their neutral-50 defaults.
//
// Thresholds:
//   - ≤1 neutral defaults → High  (nearly full data)
//   - 2–4 neutral defaults → Medium (partial data)
//   - ≥5 neutral defaults → Low   (sparse data; score is imprecise)
func computeConfidence(raw RawMetrics) ConfidenceLevel {
	n := neutralDefaultCount(raw)
	switch {
	case n <= 1:
		return ConfidenceHigh
	case n <= 4:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
