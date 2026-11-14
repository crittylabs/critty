package tui

import (
	"path/filepath"
	"strings"

	"github.com/crittylabs/critty/internal/icons"
)

func (m AppModel) iconBarButton(kind icons.Kind, button string) string {
	if !m.icons {
		return button
	}
	return modalBtnFocusedLabel.Padding(0).Render(m.icons.Glyph(kind, "")+" ") + button
}

func fileIcon(path string, binary bool) icons.Kind {
	if path == overallCommentsPath {
		return icons.Comment
	}
	if imagePath(path) {
		return icons.Image
	}
	if binary {
		return icons.Binary
	}
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case "makefile", "dockerfile", ".gitignore", ".gitattributes", ".editorconfig", ".dockerignore", ".env":
		return icons.Config
	case "gemfile", "rakefile", "cask":
		return icons.Ruby
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return icons.Go
	case ".py", ".pyi":
		return icons.Python
	case ".rs":
		return icons.Rust
	case ".rb", ".rake", ".gemspec":
		return icons.Ruby
	case ".js", ".jsx", ".mjs", ".cjs":
		return icons.JavaScript
	case ".ts", ".tsx", ".mts", ".cts":
		return icons.TypeScript
	case ".sh", ".bash", ".zsh", ".fish":
		return icons.Shell
	case ".c", ".h":
		return icons.C
	case ".cpp", ".cc", ".cxx", ".hpp":
		return icons.CPP
	case ".java":
		return icons.Java
	case ".html", ".htm":
		return icons.HTML
	case ".css", ".scss", ".sass":
		return icons.CSS
	case ".json", ".jsonc", ".json5":
		return icons.JSON
	case ".yaml", ".yml", ".toml", ".ini", ".conf", ".config", ".lock":
		return icons.Config
	case ".md", ".markdown", ".mdown":
		return icons.Markdown
	case ".el", ".ex", ".exs", ".lua", ".php", ".swift", ".kt", ".cs", ".sql":
		return icons.Code
	}
	return icons.File
}
