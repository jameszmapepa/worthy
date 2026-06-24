package score

import "testing"

func reportWithSubs(subs []SubScore) Report {
	return Report{
		Categories: []CategoryScore{{Key: "t", Label: "Test", Subs: subs}},
	}
}

func TestDrivers_OrderingAndCount(t *testing.T) {
	subs := []SubScore{
		{Key: "a", Value: 10},
		{Key: "b", Value: 90},
		{Key: "c", Value: 50},
		{Key: "d", Value: 70},
		{Key: "e", Value: 30},
		{Key: "f", Value: 100},
	}
	strong, weak := Drivers(reportWithSubs(subs))

	if len(strong) != driversN || len(weak) != driversN {
		t.Fatalf("want %d strong/weak, got %d/%d", driversN, len(strong), len(weak))
	}

	wantStrong := []string{"f", "b", "d"}
	for i, k := range wantStrong {
		if strong[i].Key != k {
			t.Errorf("strong[%d] = %q, want %q", i, strong[i].Key, k)
		}
	}
	wantWeak := []string{"a", "e", "c"}
	for i, k := range wantWeak {
		if weak[i].Key != k {
			t.Errorf("weak[%d] = %q, want %q", i, weak[i].Key, k)
		}
	}
}

func TestDrivers_StableTieBreak(t *testing.T) {
	subs := []SubScore{
		{Key: "a", Value: 50},
		{Key: "b", Value: 50},
		{Key: "c", Value: 50},
		{Key: "d", Value: 50},
	}
	strong, weak := Drivers(reportWithSubs(subs))

	if strong[0].Key != "a" || strong[1].Key != "b" || strong[2].Key != "c" {
		t.Errorf("strong tie order = %v, want a,b,c", keys(strong))
	}
	if weak[0].Key != "a" || weak[1].Key != "b" || weak[2].Key != "c" {
		t.Errorf("weak tie order = %v, want a,b,c", keys(weak))
	}
}

func TestDrivers_FewerThanN(t *testing.T) {
	subs := []SubScore{{Key: "a", Value: 10}, {Key: "b", Value: 90}}
	strong, weak := Drivers(reportWithSubs(subs))
	if len(strong) != 2 || len(weak) != 2 {
		t.Fatalf("with 2 subs want 2/2, got %d/%d", len(strong), len(weak))
	}
	if strong[0].Key != "b" || weak[0].Key != "a" {
		t.Errorf("strong[0]=%q weak[0]=%q, want b/a", strong[0].Key, weak[0].Key)
	}
}

func TestDrivers_Empty(t *testing.T) {
	strong, weak := Drivers(Report{})
	if strong != nil || weak != nil {
		t.Errorf("empty report -> nil/nil, got %v/%v", strong, weak)
	}
}

func TestDrivers_AcrossCategories(t *testing.T) {
	r := Report{Categories: []CategoryScore{
		{Key: "x", Subs: []SubScore{{Key: "a", Value: 40}, {Key: "b", Value: 60}}},
		{Key: "y", Subs: []SubScore{{Key: "c", Value: 95}, {Key: "d", Value: 5}}},
	}}
	strong, weak := Drivers(r)
	if strong[0].Key != "c" {
		t.Errorf("strongest = %q, want c (95)", strong[0].Key)
	}
	if weak[0].Key != "d" {
		t.Errorf("weakest = %q, want d (5)", weak[0].Key)
	}
}

func TestDrivers_NoOverlapWithRealReport(t *testing.T) {
	r := Evaluate(healthyRaw())
	strong, weak := Drivers(r)

	strongKeys := make(map[string]bool, len(strong))
	for _, s := range strong {
		strongKeys[s.Key] = true
	}
	for _, w := range weak {
		if strongKeys[w.Key] {
			t.Errorf("sub-score %q appears in both strong and weak sets", w.Key)
		}
	}
}

func keys(subs []SubScore) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Key
	}
	return out
}
