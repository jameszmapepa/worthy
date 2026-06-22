package tui

import "testing"

func TestGradientIndex(t *testing.T) {
	const n = 11 // gradientSteps-like span for the test
	tests := []struct {
		value float64
		want  int
	}{
		{0, 0},
		{100, n - 1},
		{50, (n - 1) / 2},
		{-10, 0},     // clamped low
		{150, n - 1}, // clamped high
	}
	for _, tc := range tests {
		if got := gradientIndex(tc.value, n); got != tc.want {
			t.Errorf("gradientIndex(%v, %d) = %d, want %d", tc.value, n, got, tc.want)
		}
	}
}

func TestGradientIndexNeverOutOfRange(t *testing.T) {
	for v := -50.0; v <= 150; v += 3 {
		i := gradientIndex(v, len(scoreGradient))
		if i < 0 || i >= len(scoreGradient) {
			t.Fatalf("gradientIndex(%v) = %d out of [0,%d)", v, i, len(scoreGradient))
		}
	}
}

func TestScoreGradientIsRedToGreen(t *testing.T) {
	if len(scoreGradient) < 3 {
		t.Fatalf("scoreGradient too short: %d", len(scoreGradient))
	}
	// The low end must read as the "poor" color and the high end as "healthy";
	// we assert they differ so the gradient is non-degenerate.
	if scoreGradient[0] == scoreGradient[len(scoreGradient)-1] {
		t.Error("gradient endpoints are identical; expected red -> green")
	}
}

func TestGradientColorClamps(t *testing.T) {
	// gradientColor must return a usable color for any input without panicking.
	_ = gradientColor(-5)
	_ = gradientColor(42)
	_ = gradientColor(1000)
}
