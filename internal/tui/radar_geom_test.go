package tui

import (
	"math"
	"testing"
)

const geomEps = 1e-9

// project maps a sub-score (value 0..100) on axis index of n total axes to a
// braille-pixel coordinate around center (cx,cy) with the given pixel radius.
// Axis 0 points straight up; axes proceed clockwise.

func TestProjectCenterAtZeroValue(t *testing.T) {
	// A zero value lands exactly on the center regardless of axis.
	for i := 0; i < 5; i++ {
		x, y := project(0, i, 5, 100, 100, 40)
		if math.Abs(x-100) > geomEps || math.Abs(y-100) > geomEps {
			t.Errorf("project(0, axis %d) = (%v,%v), want center (100,100)", i, x, y)
		}
	}
}

func TestProjectTopAxisPointsUp(t *testing.T) {
	// Axis 0 at full value points straight up: same x as center, y = cx - radius
	// (screen y grows downward, so "up" is a smaller y).
	x, y := project(100, 0, 4, 100, 100, 40)
	if math.Abs(x-100) > 1e-6 {
		t.Errorf("top axis x = %v, want 100 (no horizontal offset)", x)
	}
	if math.Abs(y-60) > 1e-6 {
		t.Errorf("top axis y = %v, want 60 (center 100 - radius 40)", y)
	}
}

func TestProjectFullValueReachesRadius(t *testing.T) {
	// A value of 100 sits exactly one radius from the center (Euclidean).
	for i := 0; i < 8; i++ {
		x, y := project(100, i, 8, 100, 100, 40)
		d := math.Hypot(x-100, y-100)
		if math.Abs(d-40) > 1e-6 {
			t.Errorf("axis %d full-value distance = %v, want 40", i, d)
		}
	}
}

func TestProjectScalesLinearlyWithValue(t *testing.T) {
	// Half the value is half the distance from center.
	x, y := project(50, 1, 6, 100, 100, 40)
	d := math.Hypot(x-100, y-100)
	if math.Abs(d-20) > 1e-6 {
		t.Errorf("half-value distance = %v, want 20", d)
	}
}

func TestProjectClockwiseFromTop(t *testing.T) {
	// With 4 axes, axis 1 is to the right (east): x > center, y ~ center.
	x, y := project(100, 1, 4, 100, 100, 40)
	if x <= 100 {
		t.Errorf("axis 1 of 4 should be east (x>100), got x=%v", x)
	}
	if math.Abs(y-100) > 1e-6 {
		t.Errorf("axis 1 of 4 should be level with center, got y=%v", y)
	}
}
