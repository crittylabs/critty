package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

func modalTextareaKeyMap() textarea.KeyMap {
	bindings := textarea.DefaultKeyMap()
	bindings.InputBegin.SetKeys(append(bindings.InputBegin.Keys(), "alt+shift+,", "alt+shift+<")...)
	bindings.InputEnd.SetKeys(append(bindings.InputEnd.Keys(), "alt+shift+.", "alt+shift+>")...)
	return bindings
}

// editHistoryKey recognizes the shared text-field and ranking shortcuts.
func editHistoryKey(msg tea.KeyPressMsg) (redo, matched bool) {
	if key.Matches(msg, keys.Redo) {
		return true, true
	}
	name := msg.String()
	return false, name == "alt+z" || isUndoKey(name)
}

// keyHintNames formats static shortcut hints without changing event names.
var keyHintNames = strings.NewReplacer(
	"ctrl+", "C-", "alt+", "M-", "shift+", "S-",
	"backspace", "DEL", "enter", "RET", "esc", "ESC", "space", "SPC", "tab", "TAB",
)

type keyMap struct {
	Redo             key.Binding
	Quit             key.Binding
	Tab              key.Binding
	Up               key.Binding
	Down             key.Binding
	HalfPageUp       key.Binding
	HalfPageDown     key.Binding
	PageUp           key.Binding
	PageDown         key.Binding
	Top              key.Binding
	Bottom           key.Binding
	NextComment      key.Binding
	PrevComment      key.Binding
	Cancel           key.Binding
	Confirm          key.Binding
	VisualMode       key.Binding
	Copy             key.Binding
	HorizontalLeft   key.Binding
	HorizontalRight  key.Binding
	HorizontalStart  key.Binding
	HorizontalEnd    key.Binding
	PrevTab          key.Binding
	NextTab          key.Binding
	NextChange       key.Binding
	PrevChange       key.Binding
	Resolve          key.Binding
	ResolveNext      key.Binding
	NextUnresolved   key.Binding
	PrevUnresolved   key.Binding
	CommentDisplay   key.Binding
	OpenAll          key.Binding
	Reload           key.Binding
	Watch            key.Binding
	IgnoreWhitespace key.Binding
	Split            key.Binding
	MarkdownSource   key.Binding
	Images           key.Binding
	Delete           key.Binding
	FileComment      key.Binding
	FormList         key.Binding
	Help             key.Binding
	FileTree         key.Binding
}

var keys = keyMap{
	Redo:             key.NewBinding(key.WithKeys("alt+_", "alt+shift+-", "alt+shift+_", "alt+Z", "alt+shift+z", "alt+shift+Z"), key.WithHelp("M-_/M-Z", "redo")),
	Quit:             key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:              key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "toggle sidebar")),
	Up:               key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("k/C-p", "up")),
	Down:             key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("j/C-n", "down")),
	HalfPageUp:       key.NewBinding(key.WithKeys("ctrl+u", "shift+up"), key.WithHelp("S-up", "half page up")),
	HalfPageDown:     key.NewBinding(key.WithKeys("ctrl+d", "shift+down"), key.WithHelp("S-down", "half page down")),
	PageUp:           key.NewBinding(key.WithKeys("alt+v", "pgup"), key.WithHelp("M-v/PgUp", "page up")),
	PageDown:         key.NewBinding(key.WithKeys("ctrl+v", "pgdown"), key.WithHelp("C-v/PgDn", "page down")),
	Top:              key.NewBinding(key.WithKeys("g", "home", "<", "alt+<", "alt+shift+,", "alt+shift+<"), key.WithHelp("</M-<", "file top")),
	Bottom:           key.NewBinding(key.WithKeys("G", "end", ">", "alt+>", "alt+shift+.", "alt+shift+>"), key.WithHelp(">/M->", "file bottom")),
	NextUnresolved:   key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next unresolved thread/required form")),
	PrevUnresolved:   key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev unresolved thread/required form")),
	NextComment:      key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next thread/form item")),
	PrevComment:      key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev thread/form item")),
	Cancel:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("ESC", "cancel")),
	Confirm:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("RET", "confirm")),
	VisualMode:       key.NewBinding(key.WithKeys("v", "ctrl+space", "ctrl+@", "ctrl+shift+@", "ctrl+shift+2"), key.WithHelp("v/C-SPC/C-@", "select lines")),
	Copy:             key.NewBinding(key.WithKeys("alt+w", "Y"), key.WithHelp("M-w/Y", "copy content")),
	HorizontalLeft:   key.NewBinding(key.WithKeys("left", "h", "ctrl+b"), key.WithHelp("h/C-b", "scroll left")),
	HorizontalRight:  key.NewBinding(key.WithKeys("right", "l", "ctrl+f"), key.WithHelp("l/C-f", "scroll right")),
	HorizontalStart:  key.NewBinding(key.WithKeys("ctrl+a", "^"), key.WithHelp("C-a/^", "scroll to left edge")),
	HorizontalEnd:    key.NewBinding(key.WithKeys("ctrl+e", "$"), key.WithHelp("C-e/$", "scroll to right edge")),
	PrevTab:          key.NewBinding(key.WithKeys("shift+tab", "alt+{", "alt+shift+[", "alt+shift+{"), key.WithHelp("S-TAB/M-{", "prev tab")),
	NextTab:          key.NewBinding(key.WithKeys("tab", "alt+}", "alt+shift+]", "alt+shift+}"), key.WithHelp("TAB/M-}", "next tab")),
	NextChange:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next hunk/item")),
	PrevChange:       key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev hunk/item")),
	Resolve:          key.NewBinding(key.WithKeys("r", "space", " "), key.WithHelp("r/SPC", "resolve/unresolve")),
	ResolveNext:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "resolve and go to next")),
	CommentDisplay:   key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "toggle reading view")),
	OpenAll:          key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open all / default")),
	Reload:           key.NewBinding(key.WithKeys("f5", "alt+r"), key.WithHelp("F5/M-r", "reload")),
	Watch:            key.NewBinding(key.WithKeys("W", "shift+w", "shift+W", "shift+f5", "alt+R", "alt+shift+r", "alt+shift+R"), key.WithHelp("W/S-F5/M-R", "watch")),
	IgnoreWhitespace: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "ignore whitespace")),
	Split:            key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "toggle split diff")),
	MarkdownSource:   key.NewBinding(key.WithKeys("alt+u"), key.WithHelp("M-u", "toggle Markdown source")),
	Images:           key.NewBinding(key.WithKeys("I", "alt+i"), key.WithHelp("I/M-i", "show/hide image, math, and Mermaid previews")),
	Delete:           key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete comment")),
	FileComment:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "comment on file")),
	FormList:         key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "answer list")),
	Help:             key.NewBinding(key.WithKeys("alt+?", "alt+shift+/", "alt+shift+?"), key.WithHelp("M-?", "help")),
	FileTree:         key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "switch sidebar view")),
}
