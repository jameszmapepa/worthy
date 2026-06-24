package score

import "testing"

func BenchmarkEvaluate(b *testing.B) {
	raw := healthyRaw()
	b.ReportAllocs()
	for b.Loop() {
		Evaluate(raw)
	}
}
