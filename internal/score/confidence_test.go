package score

import "testing"

// TestComputeConfidenceLevels asserts the three thresholds of computeConfidence
// against two representative RawMetrics instances (B4).
//
// Neutral-default count for RawMetrics{} (all-zero):
//   - CommitsLast52Weeks == nil           → +1
//   - MedianIssueFirstResponseHours == 0  → +1
//   - MergedPRs + ClosedUnmergedPRs == 0  → +1
//   - NewcomerPRsMerged + Closed == 0     → +1
//   - RecentIssuesClosed + Open == 0      → +1
//   - RecentPRsMerged + Open == 0         → +1
//   - OpenPRCount == 0                    → +1
//
// Total = 7 → ConfidenceLow (≥5 defaults).
//
// Neutral-default count for healthyRaw():
//   - OpenPRCount == 0 (field not set)    → +1
//   - all other tracked signals present   → 0 each
//
// Total = 1 → ConfidenceHigh (≤1 defaults).
func TestComputeConfidenceLevels(t *testing.T) {
	t.Run("all-zero metrics -> ConfidenceLow", func(t *testing.T) {
		// Arrange: no cohort data at all — every tracked signal falls back to neutral.
		raw := RawMetrics{}

		// Act + Assert via computeConfidence directly.
		got := computeConfidence(raw)
		if got != ConfidenceLow {
			t.Errorf("computeConfidence(empty) = %v, want Low", got)
		}
	})

	t.Run("healthy repo -> ConfidenceHigh", func(t *testing.T) {
		// Arrange: fully-populated repo with real cohort data on all tracked signals.
		raw := healthyRaw()

		// Act + Assert via computeConfidence directly.
		got := computeConfidence(raw)
		if got != ConfidenceHigh {
			t.Errorf("computeConfidence(healthy) = %v, want High", got)
		}
	})
}

// TestEvaluateSetsConfidence asserts Report.Confidence is populated by Evaluate
// so callers can read confidence without invoking computeConfidence themselves.
func TestEvaluateSetsConfidence(t *testing.T) {
	t.Run("empty metrics -> Report.Confidence == Low", func(t *testing.T) {
		// Arrange: no cohort data — all tracked signals fall back to neutral defaults.
		r := Evaluate(RawMetrics{})

		// Assert: Evaluate must propagate the confidence level onto the Report.
		if r.Confidence != ConfidenceLow {
			t.Errorf("Evaluate(empty).Confidence = %v, want Low", r.Confidence)
		}
	})

	t.Run("healthy raw -> Report.Confidence == High", func(t *testing.T) {
		// Arrange: full cohort data present; only OpenPRCount is unset (1 neutral default).
		r := Evaluate(healthyRaw())

		// Assert: one neutral default is within the ≤1 threshold for High.
		if r.Confidence != ConfidenceHigh {
			t.Errorf("Evaluate(healthy).Confidence = %v, want High", r.Confidence)
		}
	})
}
