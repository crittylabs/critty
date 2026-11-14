package tui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/crittylabs/critty/internal/icons"
)

func (m AppModel) iconBarButton(kind icons.Kind, button string) string {
	if !m.icons {
		return button
	}
	return m.icons.Join(modalBtnFocusedLabel.Padding(0).Render(m.icons.Glyph(kind, "")), button)
}

// Colors follow nvim-web-devicons 0dc0cfb0c6858041174a53c7aa4adaf65e994b3c,
// lua/nvim-web-devicons/{default,light}/icons_by_file_extension.lua.
var fileIconColors = map[icons.Kind]struct{ dark, light string }{
	icons.Go:         {"#00ADD8", "#0082A2"},
	icons.Ruby:       {"#701516", "#701516"},
	icons.Python:     {"#FFBC03", "#805E02"},
	icons.Rust:       {"#DEA584", "#6F5242"},
	icons.JavaScript: {"#CBCB41", "#666620"},
	icons.TypeScript: {"#519ABA", "#36677C"},
	icons.C:          {"#599EFF", "#3B69AA"},
	icons.CPP:        {"#519ABA", "#36677C"},
	icons.Java:       {"#CC3E44", "#992E33"},
	icons.HTML:       {"#E44D26", "#AB3A1C"},
	icons.CSS:        {"#663399", "#663399"},
	icons.Shell:      {"#4D5A5E", "#3A4446"},
	icons.Markdown:   {"#DDDDDD", "#4A4A4A"},
	icons.JSON:       {"#CBCB41", "#666620"},
	icons.Image:      {"#A074C4", "#6B4D83"},
}

func (m AppModel) renderFileTypeIcon(kind icons.Kind, style lipgloss.Style) string {
	if colors, ok := fileIconColors[kind]; ok {
		hex := colors.dark
		if !terminalIsDark {
			hex = colors.light
		}
		style = style.Foreground(lipgloss.Color(hex))
	} else if kind == icons.FolderOpen || kind == icons.FolderClosed {
		style = style.Foreground(textColors.cyan)
	}
	return style.Render(m.icons.Glyph(kind, ""))
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
