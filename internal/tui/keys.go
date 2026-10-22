package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
)

func modalTextareaKeyMap() textarea.KeyMap {
	bindings := textarea.DefaultKeyMap()
	bindings.InputBegin.SetKeys(append(bindings.InputBegin.Keys(), "alt+shift+,", "alt+shift+<")...)
	bindings.InputEnd.SetKeys(append(bindings.InputEnd.Keys(), "alt+shift+.", "alt+shift+>")...)
	return bindings
}

// keyHintNames formats static shortcut hints without changing event names.
var keyHintNames = strings.NewReplacer(
	"ctrl+", "C-", "alt+", "M-", "shift+", "S-",
	"backspace", "DEL", "enter", "RET", "esc", "ESC", "space", "SPC", "tab", "TAB",
)

type keyMap struct {
	Quit             key.Binding
	Tab              key.Binding
	Up               key.Binding
	Down             key.Binding
	HalfPageUp       key.Binding
	HalfPageDown     key.Binding
	Top              key.Binding
	Bottom           key.Binding
	NextComment      key.Binding
	PrevComment      key.Binding
	Cancel           key.Binding
	Confirm          key.Binding
	VisualMode       key.Binding
	PrevTab          key.Binding
	NextTab          key.Binding
	NextChange       key.Binding
	PrevChange       key.Binding
	Resolve          key.Binding
	ResolveNext      key.Binding
	NextUnresolved   key.Binding
	PrevUnresolved   key.Binding
	HideComments     key.Binding
	FoldResolved     key.Binding
	IgnoreWhitespace key.Binding
	Split            key.Binding
	Images           key.Binding
	Delete           key.Binding
	FileComment      key.Binding
	FormList         key.Binding
	Help             key.Binding
	FileTree         key.Binding
}

var keys = keyMap{
	Quit:             key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:              key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "toggle sidebar")),
	Up:               key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up")),
	Down:             key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down")),
	HalfPageUp:       key.NewBinding(key.WithKeys("ctrl+u", "shift+up", "pgup"), key.WithHelp("S-up", "half page up")),
	HalfPageDown:     key.NewBinding(key.WithKeys("ctrl+d", "shift+down", "pgdown"), key.WithHelp("S-down", "half page down")),
	Top:              key.NewBinding(key.WithKeys("g", "home", "<"), key.WithHelp("<", "file top")),
	Bottom:           key.NewBinding(key.WithKeys("G", "end", ">"), key.WithHelp(">", "file bottom")),
	NextUnresolved:   key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next unresolved thread/required form")),
	PrevUnresolved:   key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev unresolved thread/required form")),
	NextComment:      key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next thread/form item")),
	PrevComment:      key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev thread/form item")),
	Cancel:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("ESC", "cancel")),
	Confirm:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("RET", "confirm")),
	VisualMode:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select lines")),
	PrevTab:          key.NewBinding(key.WithKeys("shift+tab", "alt+{", "alt+shift+[", "alt+shift+{"), key.WithHelp("S-TAB/M-{", "prev tab")),
	NextTab:          key.NewBinding(key.WithKeys("tab", "alt+}", "alt+shift+]", "alt+shift+}"), key.WithHelp("TAB/M-}", "next tab")),
	NextChange:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next change/open thread/form")),
	PrevChange:       key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev change/open thread/form")),
	Resolve:          key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "resolve/unresolve")),
	ResolveNext:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "resolve and go to next")),
	HideComments:     key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "hide/show comments")),
	FoldResolved:     key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "fold resolved comments")),
	IgnoreWhitespace: key.NewBinding(key.WithKeys("w", "W"), key.WithHelp("w/W", "ignore whitespace")),
	Split:            key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "toggle split diff")),
	Images:           key.NewBinding(key.WithKeys("I"), key.WithHelp("I", "show/hide images or recheck support")),
	Delete:           key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete comment")),
	FileComment:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "comment on file")),
	FormList:         key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "answer list")),
	Help:             key.NewBinding(key.WithKeys("alt+?", "alt+shift+/", "alt+shift+?"), key.WithHelp("M-?", "help")),
	FileTree:         key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "switch sidebar view")),
}
