package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"
)

// pathDisplay caches repository boundaries, keeping filesystem access out of
// subsequent renders.  Both .git directories and linked-worktree files count.
type pathDisplay struct {
	home, cwd string
	roots     map[string]bool
}

func newPathDisplay() *pathDisplay {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	return &pathDisplay{home: home, cwd: cwd, roots: make(map[string]bool)}
}

func (p *pathDisplay) isRoot(dir string) bool {
	if p.roots == nil {
		return false
	}
	if root, ok := p.roots[dir]; ok {
		return root
	}
	info, err := os.Stat(filepath.Join(dir, ".git"))
	root := err == nil && (info.IsDir() || info.Mode().IsRegular())
	p.roots[dir] = root
	return root
}

func (p *pathDisplay) insideRepository(dir string) bool {
	for dir != "" {
		if p.isRoot(dir) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

func (p *pathDisplay) render(path string, base lipgloss.Style, tab, fileBold bool) string {
	base = base.Padding(0)
	if p == nil {
		p = &pathDisplay{}
	}
	dir := p.cwd
	home := p.home != "" && (path == p.home || strings.HasPrefix(path, p.home+string(filepath.Separator)))
	if home {
		path = "~" + strings.TrimPrefix(path, p.home)
	} else if filepath.IsAbs(path) {
		dir = filepath.VolumeName(path) + string(filepath.Separator)
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	ld := lipgloss.LightDark(terminalIsDark)
	separator := ld(lipgloss.Color("#696E78"), lipgloss.Color("#9FA6B2"))
	directory := ld(lipgloss.Color("#00535F"), lipgloss.Color("#73B8C5"))
	if !tab {
		directory = headerPathCyan
		separator = headerPathSeparator
	}
	styleFor := func(dir string) lipgloss.Style {
		style := base.Bold(false)
		if !tab && p.insideRepository(dir) {
			style = style.Background(lipgloss.Color("#D2BDDF")).Foreground(headerForeground)
		}
		return style
	}
	var rendered strings.Builder
	for i, part := range parts {
		if i > 0 {
			rendered.WriteString(styleFor(dir).Foreground(separator).Render("/"))
		}
		if home && i == 0 {
			dir = p.home
		} else {
			dir = filepath.Join(dir, part)
		}
		root := p.isRoot(dir)
		style := styleFor(dir)
		if i < len(parts)-1 {
			style = style.Foreground(directory).Bold(root)
			if tab && !root {
				short := abbreviateDirectory(part)
				style = style.Italic(short != part)
				part = short
			}
		} else {
			style = style.Bold(fileBold)
		}
		rendered.WriteString(style.Render(part))
	}
	return rendered.String()
}

func abbreviateDirectory(part string) string {
	for j, r := range part {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(part[j:], -1)
			return part[:j] + cluster
		}
	}
	return part
}
