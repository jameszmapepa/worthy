package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var helpPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorAccent).
	Padding(0, 2)

func renderHelp(width int) string {
	boxW := clampWidth(width-4, 30, 60)

	rows := []struct{ key, desc string }{
		{"← →  tab", "switch view (prev / next)"},
		{"1-4", "jump to view"},
		{"↑ ↓  j k", "move selection"},
		{"enter", "drill down (expand)"},
		{"esc", "collapse · quit when collapsed"},
		{"r", "refresh (re-fetch repo)"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Keybindings"))
	b.WriteString("\n\n")
	for _, r := range rows {
		key := mutedStyle.Render(padRight(r.key, 12))
		desc := labelStyle.Render(r.desc)
		b.WriteString(key + "  " + desc + "\n")
	}

	return helpPanelStyle.Width(boxW).Render(strings.TrimRight(b.String(), "\n"))
}

func padRight(s string, width int) string {
	n := lipgloss.Width(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
