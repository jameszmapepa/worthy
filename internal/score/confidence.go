package score

// ConfidenceLevel indicates how much real data backed a scored Report.
type ConfidenceLevel int

// ConfidenceLow, ConfidenceMedium, and ConfidenceHigh rank how much real data backed a Report.
const (
	ConfidenceLow ConfidenceLevel = iota
	ConfidenceMedium
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
