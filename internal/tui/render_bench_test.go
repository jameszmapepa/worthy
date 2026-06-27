package tui

import "testing"

func BenchmarkRenderScorecard(b *testing.B) {
	r := fixedReport()
	b.ReportAllocs()
	for b.Loop() {
		_ = renderScorecard(r, 100, -1, false)
	}
}

func BenchmarkRenderBar(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = renderBar(73.4, 28)
	}
}

func BenchmarkSparkline(b *testing.B) {
	series := fixedRaw().CommitsLast52Weeks
	b.ReportAllocs()
	for b.Loop() {
		_ = sparklineBlock(series, 40)
	}
}
