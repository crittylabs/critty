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
)

// Codepoints from Nerd Fonts v3.4.0's glyphnames.json (Codicons, Seti, Devicons).
var glyphs = [...]string{
	FolderClosed: "\uea83", FolderOpen: "\ueaf7", File: "\uea7b", Code: "\ueae9",
	Binary: "\ueae8", Image: "\ueaea", Config: "\ue615", JSON: "\ue60b",
	Markdown: "\ue609", Go: "\ue627", Python: "\ue73c", Rust: "\ue7a8",
	Ruby: "\ue739", JavaScript: "\ue781", TypeScript: "\ue628", Shell: "\ue691",
	C: "\ue649", CPP: "\ue646", Java: "\ue66d", HTML: "\ue60e", CSS: "\ue614",
	Added: "\ueadc", Modified: "\ueade", Removed: "\ueadf", Renamed: "\ueae0",
	Comment: "\uea6b",
}

func (s Set) Glyph(kind Kind, fallback string) string {
	if !s || int(kind) >= len(glyphs) {
		return fallback
	}
	return glyphs[kind]
}
