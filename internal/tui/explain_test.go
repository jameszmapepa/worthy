package tui

import (
	"strings"
	"testing"
)

func TestExplainShowsVerdict(t *testing.T) {
	out := renderExplain(fixedReport(), 80)
	if !strings.Contains(out, "In fair health") {
		t.Errorf("explain view should show the verdict headline:\n%s", out)
	}
}

func TestExplainShowsDrivers(t *testing.T) {
	out := renderExplain(fixedReport(), 80)
	for _, w := range []string{"Strongest", "Weakest", "License", "Workflow safety", "Release cadence"} {
		if !strings.Contains(out, w) {
			t.Errorf("explain view missing driver content %q in:\n%s", w, out)
		}
	}
}

func TestExplainShowsGateGuidance(t *testing.T) {
	out := renderExplain(fixedReport(), 80)
	if !strings.Contains(out, "Closed to newcomers") {
		t.Error("explain view should show triggered gate titles")
	}
	if !strings.Contains(out, "Merge PRs from first-time") {
		t.Error("explain view should show each gate's HowToClear guidance")
	}
}

func TestExplainHealthyState(t *testing.T) {
	out := renderExplain(healthyFixedReport(), 80)
	if !strings.Contains(out, "No gates triggered") {
		t.Errorf("healthy repo should show the no-gates state:\n%s", out)
	}
	if strings.Contains(out, "Closed to newcomers") {
		t.Error("healthy repo must not show any gate")
	}
}

func TestFourthViewSelectableByKey(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "4")
	if m.view != 3 {
		t.Errorf("key 4 -> view %d, want 3", m.view)
	}
}

func TestExplainRendersAsActiveView(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "4")
	out := m.render()
	if !strings.Contains(out, "In fair health") {
		t.Error("view 3 should render the Explain view with the verdict")
	}
}

func TestFooterShowsExplainTab(t *testing.T) {
	m := loadedModel(t)
	if !strings.Contains(m.renderFooter(), "Explain") {
		t.Error("footer should list the Explain tab")
	}
}

func TestExplainFallsBackToGradeWhenVerdictEmpty(t *testing.T) {
	r := fixedReport()
	r.Verdict = ""
	out := renderExplain(r, 80)
	if !strings.Contains(out, "Grade") {
		t.Errorf("explain view should fall back to Grade label when Verdict is empty:\n%s", out)
	}
}
