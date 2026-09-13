package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKeymapTopBottomMoveSelection(t *testing.T) {
	m := loadedModel(t)
	m = press(m, "G")
	if want := m.currentSelectableCount() - 1; m.selected != want {
		t.Errorf("G selected = %d, want %d", m.selected, want)
	}
	m = press(m, "g")
	if m.selected != 0 {
		t.Errorf("g selected = %d, want 0", m.selected)
	}
}

func TestKeymapOpenUsesInjectedOpener(t *testing.T) {
	m := loadedModel(t)
	var got string
	m.opener = func(u string) error { got = u; return nil }
	_, cmd := m.Update(keyPress("o"))
	if cmd == nil {
		t.Fatal("o should return a command")
	}
	msg := cmd()
	if got != "https://github.com/charmbracelet/bubbletea" {
		t.Errorf("opened %q", got)
	}
	st, ok := msg.(statusMsg)
	if !ok || !strings.Contains(st.text, "Opened") {
		t.Errorf("open should report a status, got %#v", msg)
	}
}

func TestKeymapCopySetsStatusAndExpires(t *testing.T) {
	m := loadedModel(t)
	next, cmd := m.Update(keyPress("y"))
	got := next.(Model)
	if cmd == nil {
		t.Fatal("y should return a command")
	}
	if !strings.Contains(stripANSI(got.renderFooter()), "Copied summary") {
		t.Errorf("footer should show the copy status:\n%s", got.renderFooter())
	}
	next, _ = got.Update(statusExpiredMsg{gen: got.statusGen})
	if next.(Model).status != "" {
		t.Error("status should clear when its timer fires")
	}
	next, _ = got.Update(statusExpiredMsg{gen: got.statusGen - 1})
	if next.(Model).status == "" {
		t.Error("a stale expiry must not clear a newer status")
	}
}

func TestSummaryLine(t *testing.T) {
	m := loadedModel(t)
	s := m.summary()
	for _, want := range []string{"charmbracelet/bubbletea", "Grade C", "68.2/100", "https://github.com/charmbracelet/bubbletea"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q: %s", want, s)
		}
	}
}

func TestFooterAndHelpComeFromKeymap(t *testing.T) {
	m := loadedModel(t)
	footer := stripANSI(m.renderFooter())
	for _, want := range []string{"select", "drill down", "switch view", "quit"} {
		if !strings.Contains(footer, want) {
			t.Errorf("footer missing %q:\n%s", want, footer)
		}
	}
	m.helpVisible = true
	overlay := stripANSI(m.renderHelp())
	for _, want := range []string{"open on GitHub", "copy summary", "first / last", "scroll"} {
		if !strings.Contains(overlay, want) {
			t.Errorf("help missing %q:\n%s", want, overlay)
		}
	}
}

func TestShiftGAlsoJumpsToBottom(t *testing.T) {
	m := loadedModel(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModShift, Text: "G"})
	if got := next.(Model).selected; got != m.currentSelectableCount()-1 {
		t.Errorf("shift+g selected = %d", got)
	}
}
