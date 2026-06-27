package tui

import "testing"

// BenchmarkRenderScorecard measures a full scorecard render: two question
// headline cards, the composite hero, three category panels of indicator bars,
// and gate badges. This is the per-keypress cost during navigation, so it is
// the most user-visible render path.
func BenchmarkRenderScorecard(b *testing.B) {
	r := fixedReport()
	b.ReportAllocs()
	for b.Loop() {
		_ = renderScorecard(r, 100, -1, false)
	}
}

// BenchmarkRenderBar measures the single-bar gradient render. The run-length
// implementation groups adjacent cells that share a gradient color so the cost
// is bounded by the number of gradient stops, not the bar width.
func BenchmarkRenderBar(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = renderBar(73.4, 28)
	}
}

// BenchmarkSparkline measures the dependency-free block-character sparkline that
// replaced ntcharts.
func BenchmarkSparkline(b *testing.B) {
	series := fixedRaw().CommitsLast52Weeks
	b.ReportAllocs()
	for b.Loop() {
		_ = sparklineBlock(series, 40)
	}
}
