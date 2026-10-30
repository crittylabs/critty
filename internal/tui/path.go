package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	return p.repositoryRoot(dir) != ""
}

func (p *pathDisplay) repositoryRoot(dir string) string {
	for dir != "" {
		if p.isRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func (p *pathDisplay) treePath(path string) string {
	if p == nil || p.cwd == "" || !filepath.IsAbs(path) {
		return path
	}
	base := p.cwd
	if root := p.repositoryRoot(base); root != "" {
		base = root
	}
	relative, err := filepath.Rel(base, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(relative)
}

func (p *pathDisplay) fileRoot(path string) string {
	if p == nil || p.cwd == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.cwd, path)
	}
	if root := p.repositoryRoot(filepath.Dir(path)); root != "" {
		return root
	}
	if within(p.cwd, path) {
		return p.cwd
	}
	return filepath.Dir(path)
}

func rootLabel(root string, roots []string) string {
	label := filepath.Base(root)
	for parent := filepath.Dir(root); ; parent = filepath.Dir(parent) {
		duplicate := false
		for _, other := range roots {
			if other != root && strings.HasSuffix(filepath.ToSlash(other), "/"+label) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			return label
		}
		if filepath.Dir(parent) == parent {
			return filepath.ToSlash(root)
		}
		label = filepath.ToSlash(filepath.Join(filepath.Base(parent), label))
	}
}

func tabPathStyles(base lipgloss.Style) (lipgloss.Style, lipgloss.Style) {
	ld := lipgloss.LightDark(terminalIsDark)
	base = base.Padding(0)
	return base.Foreground(ld(lipgloss.Color("#00535F"), lipgloss.Color("#73B8C5"))),
		base.Foreground(ld(lipgloss.Color("#696E78"), lipgloss.Color("#9FA6B2")))
}

func (p *pathDisplay) renderTab(path string, base lipgloss.Style, bold bool, roots []string, width int) string {
	if len(roots) < 2 && !filepath.IsAbs(path) {
		return p.renderWithin(path, base, true, bold, width)
	}
	root := p.fileRoot(path)
	if root == "" {
		return p.renderWithin(path, base, true, bold, width)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.cwd, path)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return p.renderWithin(path, base, true, bold, width)
	}
	formatter := *p
	formatter.cwd = root
	if len(roots) < 2 {
		return formatter.renderWithin(relative, base, true, bold, width)
	}
	directory, separator := tabPathStyles(base)
	parts := strings.Split(rootLabel(root, roots), "/")
	for i, part := range parts {
		parts[i] = directory.Bold(true).Render(part)
	}
	slash := separator.Bold(false).Render("/")
	prefix := strings.Join(parts, slash) + slash
	if width > 0 && width < lipgloss.Width(prefix)+lipgloss.Width(filepath.Base(relative)) {
		return formatter.renderWithin(relative, base, true, bold, width)
	}
	if width > 0 {
		width = max(1, width-lipgloss.Width(prefix))
	}
	return prefix + formatter.renderWithin(relative, base, true, bold, width)
}

func (p *pathDisplay) render(path string, base lipgloss.Style, tab, fileBold bool) string {
	return p.renderWithin(path, base, tab, fileBold, 0)
}

func (p *pathDisplay) renderWithin(path string, base lipgloss.Style, tab, fileBold bool, width int) string {
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
	display := parts
	if tab {
		display = p.tabPathParts(parts, dir, home, width)
	}
	directoryStyle, separatorStyle := tabPathStyles(base)
	separator := separatorStyle.GetForeground()
	directory := directoryStyle.GetForeground()
	if !tab {
		directory = headerPathCyan
		separator = headerPathSeparator
	}
	styleFor := func(dir string) lipgloss.Style {
		inside := !tab && (p.insideRepository(dir) || p.cwd != "" && within(p.cwd, dir))
		style := base.Bold(inside)
		if !tab && p.insideRepository(dir) {
			style = style.Background(lipgloss.Color("#D2BDDF")).Foreground(headerForeground)
		}
		return style
	}
	var rendered strings.Builder
	if !tab {
		rendered.WriteString(ansi.NewStyle().Normal().String())
	}
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
			style = style.Foreground(directory).Bold(root || style.GetBold())
			if tab {
				short := display[i]
				style = style.Italic(short != part)
				part = short
			}
		} else {
			style = style.Bold(fileBold)
		}
		rendered.WriteString(style.Render(part))
	}
	if tab && width > 0 {
		filename := parts[len(parts)-1]
		if nameWidth := lipgloss.Width(filename); width >= nameWidth && width < nameWidth+2 {
			return base.Bold(fileBold).Render(filename)
		}
		return truncateLeftToWidth(rendered.String(), width)
	}
	return rendered.String()
}

func (p *pathDisplay) tabPathParts(parts []string, dir string, home bool, width int) []string {
	display := append([]string(nil), parts...)
	for i, part := range parts[:len(parts)-1] {
		if home && i == 0 {
			dir = p.home
		} else {
			dir = filepath.Join(dir, part)
		}
		if !p.isRoot(dir) {
			display[i] = abbreviateDirectory(part)
		}
	}
	remaining := width - lipgloss.Width(strings.Join(display, "/"))
	for i := len(parts) - 2; i >= 0 && remaining > 0; i-- {
		current := lipgloss.Width(display[i])
		expanded := directoryPrefix(parts[i], current+remaining)
		remaining -= lipgloss.Width(expanded) - current
		display[i] = expanded
	}
	return display
}

func directoryPrefix(part string, width int) string {
	graphemes := uniseg.NewGraphemes(part)
	used, end := 0, 0
	for graphemes.Next() {
		next := lipgloss.Width(graphemes.Str())
		if used+next > width {
			break
		}
		used += next
		_, end = graphemes.Positions()
	}
	return part[:end]
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
