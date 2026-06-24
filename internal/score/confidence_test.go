package score

import "testing"

func TestComputeConfidenceLevels(t *testing.T) {
	t.Run("all-zero metrics -> ConfidenceLow", func(t *testing.T) {
		raw := RawMetrics{}

		got := computeConfidence(raw)
		if got != ConfidenceLow {
			t.Errorf("computeConfidence(empty) = %v, want Low", got)
		}
	})

	t.Run("healthy repo -> ConfidenceHigh", func(t *testing.T) {
		raw := healthyRaw()

		got := computeConfidence(raw)
		if got != ConfidenceHigh {
			t.Errorf("computeConfidence(healthy) = %v, want High", got)
		}
	})
}

func TestEvaluateSetsConfidence(t *testing.T) {
	t.Run("empty metrics -> Report.Confidence == Low", func(t *testing.T) {
		r := Evaluate(RawMetrics{})

		if r.Confidence != ConfidenceLow {
			t.Errorf("Evaluate(empty).Confidence = %v, want Low", r.Confidence)
		}
	})

	t.Run("healthy raw -> Report.Confidence == High", func(t *testing.T) {
		r := Evaluate(healthyRaw())

		if r.Confidence != ConfidenceHigh {
			t.Errorf("Evaluate(healthy).Confidence = %v, want High", r.Confidence)
		}
	})
}
