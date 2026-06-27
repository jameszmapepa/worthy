package tui

import "testing"

const maxWideBarAllocs = 200

func TestRenderBarAllocationsBoundedByGradient(t *testing.T) {
	const wide = 400

	wideAllocs := testing.AllocsPerRun(200, func() { _ = renderBar(73.4, wide) })
	if wideAllocs > maxWideBarAllocs {
		t.Errorf("renderBar(width=%d) = %.0f allocs, want <= %d "+
			"(per-cell regression scales with width)", wide, wideAllocs, maxWideBarAllocs)
	}

	narrowAllocs := testing.AllocsPerRun(200, func() { _ = renderBar(73.4, 28) })
	if delta := wideAllocs - narrowAllocs; delta > float64(len(scoreGradient)) {
		t.Errorf("renderBar allocs grew by %.0f from width 28->%d; want <= %d "+
			"(gradient-stop bound) — allocations are scaling with width", delta, wide, len(scoreGradient))
	}
}

const maxSparklineAllocs = 8

func TestSparklineAllocationsBounded(t *testing.T) {
	long := make([]int, 520)
	for i := range long {
		long[i] = i % 13
	}
	got := testing.AllocsPerRun(200, func() { _ = sparklineBlock(long, 40) })
	if got > maxSparklineAllocs {
		t.Errorf("sparklineBlock(len=%d) = %.0f allocs, want <= %d "+
			"(cost must not scale with series length)", len(long), got, maxSparklineAllocs)
	}
}
