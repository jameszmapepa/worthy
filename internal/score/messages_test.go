package score

import "testing"

// TestQuestionMessageEveryGrade pins the diplomatic per-question reading for
// each question and grade, so a wording change is a deliberate, reviewed edit.
func TestQuestionMessageEveryGrade(t *testing.T) {
	cases := []struct {
		key, grade, want string
	}{
		{"maintained", "A", "Actively maintained"},
		{"maintained", "C", "Steady but slowing"},
		{"maintained", "F", "Largely inactive"},
		{"newcomer", "A", "Welcoming to newcomers"},
		{"newcomer", "D", "Selective on outside PRs"},
		{"newcomer", "F", "Rarely merges outside PRs"},
	}
	for _, c := range cases {
		if got := questionMessage(c.key, c.grade); got != c.want {
			t.Errorf("questionMessage(%q,%q) = %q, want %q", c.key, c.grade, got, c.want)
		}
	}

	// Every A-F grade must have a message for both questions (no blank cards).
	for _, key := range []string{"maintained", "newcomer"} {
		for _, g := range []string{"A", "B", "C", "D", "F"} {
			if questionMessage(key, g) == "" {
				t.Errorf("missing message for %s grade %s", key, g)
			}
		}
	}

	// Unknown inputs degrade to empty, not a panic.
	if questionMessage("maintained", "Z") != "" || questionMessage("bogus", "A") != "" {
		t.Error("unknown key/grade should yield an empty message")
	}
}

// TestEvaluatePopulatesQuestionMessages proves Evaluate wires the message onto
// both first-class question scores, matching their own grade.
func TestEvaluatePopulatesQuestionMessages(t *testing.T) {
	r := Evaluate(healthyRaw())
	if r.Maintained.Message != questionMessage("maintained", r.Maintained.Grade) {
		t.Errorf("Maintained.Message = %q; want match for grade %s", r.Maintained.Message, r.Maintained.Grade)
	}
	if r.Contributable.Message != questionMessage("newcomer", r.Contributable.Grade) {
		t.Errorf("Contributable.Message = %q; want match for grade %s", r.Contributable.Message, r.Contributable.Grade)
	}
	if r.Maintained.Message == "" || r.Contributable.Message == "" {
		t.Error("a healthy repo must still carry both question messages")
	}
}

// TestVerdictUsesRewordedOpeners pins the diplomatic, two-question-aware grade
// openers used by buildVerdict.
func TestVerdictUsesRewordedOpeners(t *testing.T) {
	want := map[string]string{
		"A": "Healthy and welcoming",
		"B": "In good shape",
		"C": "Mixed signals",
		"D": "Some concerns",
		"F": "Serious concerns",
	}
	for grade, opener := range want {
		if got := gradePhrase[grade]; got != opener {
			t.Errorf("gradePhrase[%q] = %q, want %q", grade, got, opener)
		}
	}
}
