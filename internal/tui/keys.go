package tui

import "charm.land/bubbles/v2/key"

// KeyMap is every binding the browser responds to. List navigation keys are
// handled by the list bubble itself and only appear here for the help view.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Top       key.Binding
	Bottom    key.Binding
	HalfUp    key.Binding
	HalfDown  key.Binding
	Switch    key.Binding
	Filter    key.Binding
	Jump      key.Binding
	Clear     key.Binding
	Annotated key.Binding
	Compact   key.Binding
	Raw       key.Binding
	JSON      key.Binding
	XML       key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
	Copy      key.Binding
	Help      key.Binding
	Quit      key.Binding
}

func defaultKeyMap() KeyMap {
	return KeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:       key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "half page up")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half page down")),
		Switch:    key.NewBinding(key.WithKeys("tab", "enter"), key.WithHelp("tab", "switch pane")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Jump:      key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "jump to record")),
		Clear:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
		Annotated: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "annotated")),
		Compact:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compact")),
		Raw:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "raw")),
		JSON:      key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "json")),
		XML:       key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "xml")),
		NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "previous match")),
		Copy:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy record")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp and FullHelp satisfy help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.NextMatch, k.Jump, k.Switch, k.Compact, k.Copy, k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.HalfUp, k.HalfDown, k.Switch},
		{k.Filter, k.Jump, k.Clear},
		{k.NextMatch, k.PrevMatch},
		{k.Annotated, k.Compact, k.Raw, k.JSON, k.XML},
		{k.Copy, k.Help, k.Quit},
	}
}
