package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	Next     key.Binding
	Prev     key.Binding
	Jump     key.Binding
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Enter    key.Binding
	Back     key.Binding
	PageDown key.Binding
	PageUp   key.Binding
	Refresh  key.Binding
	Open     key.Binding
	Copy     key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Next:     key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("←→", "switch view")),
		Prev:     key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "previous view")),
		Jump:     key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "jump to view")),
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "select")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j/k", "select")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/G", "first / last")),
		Bottom:   key.NewBinding(key.WithKeys("G", "shift+g", "end"), key.WithHelp("G", "last")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "drill down")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "collapse")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d", "space"), key.WithHelp("pgdn/pgup", "scroll")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("ctrl+u/d", "scroll")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Open:     key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open on GitHub")),
		Copy:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy summary")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (m Model) shortHelp() []key.Binding {
	k := m.keymap()
	switch {
	case m.helpVisible:
		return []key.Binding{k.Help, k.Quit}
	case m.canSelect() && m.expanded:
		return []key.Binding{k.Back, k.Next, k.Refresh, k.Open, k.Quit}
	case m.canSelect():
		return []key.Binding{k.Up, k.Enter, k.Next, k.Refresh, k.Help, k.Quit}
	case m.state == stateLoaded:
		return []key.Binding{k.Next, k.PageDown, k.Refresh, k.Help, k.Quit}
	default:
		return []key.Binding{k.Next, k.Refresh, k.Help, k.Quit}
	}
}

func (m Model) fullHelp() [][]key.Binding {
	k := m.keymap()
	return [][]key.Binding{
		{k.Next, k.Prev, k.Jump},
		{k.Up, k.Top, k.Enter, k.Back},
		{k.PageDown, k.PageUp, k.Refresh},
		{k.Open, k.Copy, k.Help, k.Quit},
	}
}

func helpModel(width int) help.Model {
	h := help.New()
	h.SetWidth(width)
	h.Styles.ShortKey = labelStyle
	h.Styles.ShortDesc = mutedStyle
	h.Styles.ShortSeparator = mutedStyle
	h.Styles.Ellipsis = mutedStyle
	h.Styles.FullKey = labelStyle
	h.Styles.FullDesc = mutedStyle
	h.Styles.FullSeparator = mutedStyle
	return h
}
