package score

import "testing"

// reportWith builds a minimal Report with the three categories set to the given
// values and their canonical composite weights, for verdict-aggregation tests.
func reportWith(activity, community, security float64) Report {
	return Report{Categories: []CategoryScore{
		{Key: CategoryActivity, Label: "Activity", Value: activity, Weight: weightActivity},
		{Key: CategoryCommunity, Label: "Community", Value: community, Weight: weightCommunity},
		{Key: CategorySecurity, Label: "Security", Value: security, Weight: weightSecurity},
	}}
}

func TestQuestionVerdictsOrderAndKeys(t *testing.T) {
	// Arrange
	r := reportWith(90, 60, 70)

	// Act
	got := QuestionVerdicts(r)

	// Assert
	if len(got) != 2 {
		t.Fatalf("QuestionVerdicts len = %d, want 2", len(got))
	}
	if got[0].Key != "maintained" || got[1].Key != "newcomer" {
		t.Errorf("verdict order = %q,%q; want maintained,newcomer", got[0].Key, got[1].Key)
	}
	if got[0].Label != "Will it last?" || got[1].Label != "Will my PR land?" {
		t.Errorf("labels = %q,%q", got[0].Label, got[1].Label)
	}
	wantMaintainedCats := []string{CategoryActivity, CategorySecurity}
	if !equalStrings(got[0].CategoryKeys, wantMaintainedCats) {
		t.Errorf("maintained categories = %v, want %v", got[0].CategoryKeys, wantMaintainedCats)
	}
	if !equalStrings(got[1].CategoryKeys, []string{CategoryCommunity}) {
		t.Errorf("newcomer categories = %v, want [community]", got[1].CategoryKeys)
	}
}

func TestQuestionVerdictsWeightedMaintained(t *testing.T) {
	// Arrange: activity 90 (w .40), security 70 (w .30) -> (36+21)/0.70 = 81.4
	r := reportWith(90, 60, 70)

	// Act
	got := QuestionVerdicts(r)

	// Assert
	if got[0].Value != 81.4 {
		t.Errorf("maintained value = %.2f, want 81.4", got[0].Value)
	}
	if got[0].Grade != "B" {
		t.Errorf("maintained grade = %q, want B", got[0].Grade)
	}
}

func TestQuestionVerdictsNewcomerIsCommunity(t *testing.T) {
	// Arrange
	r := reportWith(90, 60, 70)

	// Act
	got := QuestionVerdicts(r)

	// Assert: newcomer == community value (60) -> grade C (>=55)
	if got[1].Value != 60 {
		t.Errorf("newcomer value = %.2f, want 60", got[1].Value)
	}
	if got[1].Grade != "C" {
		t.Errorf("newcomer grade = %q, want C", got[1].Grade)
	}
}

func TestQuestionVerdictsEmptyReport(t *testing.T) {
	// Act
	got := QuestionVerdicts(Report{})

	// Assert: no categories -> zero values, F grade, empty category lists, no panic
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, q := range got {
		if q.Value != 0 {
			t.Errorf("%s value = %.2f, want 0", q.Key, q.Value)
		}
		if q.Grade != "F" {
			t.Errorf("%s grade = %q, want F", q.Key, q.Grade)
		}
		if len(q.CategoryKeys) != 0 {
			t.Errorf("%s category keys = %v, want empty", q.Key, q.CategoryKeys)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
