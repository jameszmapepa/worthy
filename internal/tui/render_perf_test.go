package tui

import "testing"

// These are STRICT allocation-ceiling tests (deterministic, unlike timing): they
// fail the build if a hot render path regresses its allocation behaviour. They
// guard the optimizations the audit introduced, not just measure them.

// maxWideBarAllocs bounds renderBar's allocations at a deliberately wide width.
// The run-length implementation renders one styled string per gradient-color
// RUN, so its cost is capped by the number of gradient stops (~two dozen),
// independent of bar width. A regression to the old one-style-per-filled-cell
// behaviour would scale with width: at width 400 it would allocate ~400+, well
// above this ceiling. Measured ~161 at any width >= 80; 200 leaves headroom for
// lipgloss patch variance while staying far below the per-cell failure mode.
const maxWideBarAllocs = 200

// TestRenderBarAllocationsBoundedByGradient pins the run-length invariant:
// allocations do not scale with bar width.
func TestRenderBarAllocationsBoundedByGradient(t *testing.T) {
	const wide = 400

	wideAllocs := testing.AllocsPerRun(200, func() { _ = renderBar(73.4, wide) })
	if wideAllocs > maxWideBarAllocs {
		t.Errorf("renderBar(width=%d) = %.0f allocs, want <= %d "+
			"(per-cell regression scales with width)", wide, wideAllocs, maxWideBarAllocs)
	}

	// Direct non-scaling check: a 14x wider bar must not cost meaningfully more.
	narrowAllocs := testing.AllocsPerRun(200, func() { _ = renderBar(73.4, 28) })
	if delta := wideAllocs - narrowAllocs; delta > float64(len(scoreGradient)) {
		t.Errorf("renderBar allocs grew by %.0f from width 28->%d; want <= %d "+
			"(gradient-stop bound) — allocations are scaling with width", delta, wide, len(scoreGradient))
	}
}

// maxSparklineAllocs bounds the dependency-free sparkline. It normalizes into a
// fixed-width []rune and emits one styled string, so cost is independent of the
// input series length. Measured 5; 8 leaves a little headroom.
const maxSparklineAllocs = 8

// TestSparklineAllocationsBounded pins that sparkline cost does not scale with
// the length of the input series.
func TestSparklineAllocationsBounded(t *testing.T) {
	long := make([]int, 520) // 10x a year of weekly commits
	for i := range long {
		long[i] = i % 13
	}
	got := testing.AllocsPerRun(200, func() { _ = sparklineBlock(long, 40) })
	if got > maxSparklineAllocs {
		t.Errorf("sparklineBlock(len=%d) = %.0f allocs, want <= %d "+
			"(cost must not scale with series length)", len(long), got, maxSparklineAllocs)
	}
}
