package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestThemeSwitchRebuildsStyles(t *testing.T) {
	t.Cleanup(func() { setTheme(true) })

	darkTitle := titleStyle.Render("x")
	darkBar := renderBar(90, 10)
	setTheme(false)
	if currentPaletteDark {
		t.Fatal("setTheme(false) should select the light palette")
	}
	if titleStyle.Render("x") == darkTitle {
		t.Error("title style should change with the palette")
	}
	if renderBar(90, 10) == darkBar {
		t.Error("score gradient should be rebuilt for the light palette")
	}
	if got := lipgloss.NewStyle().Foreground(colorAmber).Render("a"); !strings.Contains(got, "178;106;0") {
		t.Errorf("light amber should be the readable #b26a00, got %q", got)
	}
	setTheme(true)
	if titleStyle.Render("x") != darkTitle {
		t.Error("restoring the dark palette should restore the original styles")
	}
}

func TestBackgroundColorMsgSelectsTheme(t *testing.T) {
	t.Cleanup(func() { setTheme(true) })
	m := newTestModel()
	if _, _ = m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")}); currentPaletteDark {
		t.Error("a light background should select the light palette")
	}
	if _, _ = m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")}); !currentPaletteDark {
		t.Error("a dark background should select the dark palette")
	}
}

func TestGateBadgeInkStaysDarkInLightTheme(t *testing.T) {
	t.Cleanup(func() { setTheme(true) })
	setTheme(false)
	out := renderGateBadge(fixedReport().Gates[0])
	if !strings.Contains(out, "40;42;54") {
		t.Errorf("badge ink should remain #282a36 on a light theme, got %q", out)
	}
}
