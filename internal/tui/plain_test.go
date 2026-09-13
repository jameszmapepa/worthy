package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderPlainContainsScorecardAndExplanation(t *testing.T) {
	out := RenderPlain("charm", "bubbletea", fixedReport(), fixedRaw(), PlainOptions{Width: 100})
	for _, want := range []string{"charm/bubbletea", "68.2", "Activity", "Strongest", "Gates"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain output missing %q", want)
		}
	}
	if strings.Contains(out, selectionMarker) {
		t.Error("plain output must not carry a selection marker")
	}
	for i, line := range strings.Split(out, "\n") {
		if lw := lipgloss.Width(line); lw > 100 {
			t.Errorf("line %d is %d wide", i, lw)
		}
	}
}

func TestRenderPlainDefaultsWidth(t *testing.T) {
	out := RenderPlain("o", "r", fixedReport(), fixedRaw(), PlainOptions{})
	if out == "" {
		t.Fatal("empty output")
	}
}
