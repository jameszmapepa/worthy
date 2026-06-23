package score

import "testing"

// BenchmarkEvaluate measures the pure scoring path: five activity sub-scores,
// five community, four security, the weighted composite, and all conditional
// gates. It is a Small benchmark (no I/O), so the numbers are stable CPU time.
func BenchmarkEvaluate(b *testing.B) {
	raw := healthyRaw()
	b.ReportAllocs()
	for b.Loop() {
		Evaluate(raw)
	}
}
