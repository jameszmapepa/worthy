package tui

import "strings"

// wideHelpWidth is the text width at which all four help columns fit.
const wideHelpWidth = 92

func (m Model) renderHelp() string {
	boxW := clampWidth(m.width-4, 30, 100)
	textW := panelTextWidth(boxW)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Keybindings"))
	b.WriteString("\n\n")
	groups := m.fullHelp()
	h := helpModel(textW)
	if textW >= wideHelpWidth {
		b.WriteString(h.FullHelpView(groups))
	} else {
		// Narrow: two rows of two groups so no column is elided.
		b.WriteString(h.FullHelpView(groups[:2]))
		b.WriteString("\n\n")
		b.WriteString(h.FullHelpView(groups[2:]))
	}
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("esc also quits when nothing is expanded."))

	return helpPanelStyle.Width(boxW).Render(b.String())
}
