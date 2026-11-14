// Package icons selects Nerd Font glyphs with caller-owned portable fallbacks.
package icons

type Set bool

const (
	Plain Set = false
	Nerd  Set = true
)

type Kind uint8

const (
	FolderClosed Kind = iota
	FolderOpen
	File
	Code
	Binary
	Image
	Config
	JSON
	Markdown
	Go
	Python
	Rust
	Ruby
	JavaScript
	TypeScript
	Shell
	C
	CPP
	Java
	HTML
	CSS
	Added
	Modified
	Removed
	Renamed
	Comment
	Unchecked
	Checked
	Reply
	Warning
	Outline
	Function
	Method
	Class
	Struct
	Interface
	Enum
	Namespace
	Constant
	Variable
	Heading
	Project
	Branch
	Watch
	Copy
	Edit
	Back
	Forward
	Close
	Send
	Save
	External
	GoTo
	Delete
	Suggest
	RadioUnchecked
	RadioChecked
	Mixed
	Check
	TabLeft
	TabRight
)

// Codepoints from Nerd Fonts v3.4.0's glyphnames.json.
var glyphs = [...]string{
	FolderClosed: "\uea83", FolderOpen: "\ueaf7", File: "\uea7b", Code: "\ueae9",
	Binary: "\ueae8", Image: "\ueaea", Config: "\ue615", JSON: "\ue60b",
	Markdown: "\uf48a", Go: "\ue627", Python: "\ue73c", Rust: "\ue7a8",
	Ruby: "\ue739", JavaScript: "\ue781", TypeScript: "\ue628", Shell: "\ue691",
	C: "\ue649", CPP: "\ue646", Java: "\ue66d", HTML: "\ue60e", CSS: "\ue614",
	Added: "\ueadc", Modified: "\ueade", Removed: "\ueadf", Renamed: "\ueae0",
	Comment:   "\uea6b",
	Unchecked: "\U000f0131", Checked: "\U000f0132", Reply: "\uea7d", Warning: "\uea6c", Outline: "\ueb86",
	Function: "\U000f0295", Method: "\uea8c", Class: "\ueb5b", Struct: "\uea91",
	Interface: "\ueb61", Enum: "\uea95", Namespace: "\uea8b", Constant: "\ueb5d",
	Variable: "\uea88", Heading: "\uf48a",
	Project: "\ueb30", Branch: "\uf418", Watch: "\uea70",
	Copy: "\uebcc", Edit: "\uea73", Back: "\uea9b", Forward: "\uea9c", Close: "\uea76",
	Send: "\uec0f", Save: "\ueb4b", External: "\ueb14", GoTo: "\uea94", Delete: "\uea81", Suggest: "\uea61",
	RadioUnchecked: "\U000f0130", RadioChecked: "\U000f043e", Mixed: "\U000f0856", Check: "\ueab2",
	TabLeft: "\ue0b2", TabRight: "\ue0b0",
}

// Glyph reserves two cells for Nerd Font glyphs, which may overhang one cell.
// Close buttons, Powerline tab edges, and portable fallbacks keep their width.
func (s Set) Glyph(kind Kind, fallback string) string {
	if !s || int(kind) >= len(glyphs) {
		return fallback
	}
	if kind == Close || kind == TabLeft || kind == TabRight {
		return glyphs[kind]
	}
	return glyphs[kind] + " "
}

// IsTabEdge identifies shapes painted with a tab's background color.
func IsTabEdge(glyph string) bool {
	return glyph == glyphs[TabLeft] || glyph == glyphs[TabRight]
}

func (s Set) Label(kind Kind, label string) string {
	if !s {
		return label
	}
	return s.Glyph(kind, "") + " " + label
}
