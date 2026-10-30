package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func TestPathDisplayRepositoryBoundaries(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "src", "atlas")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGitIn(t, root, "init", "-q")
	runGitIn(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	worktree := filepath.Join(home, "work", "atlas.fix")
	nested := filepath.Join(root, ".trees", "feature")
	for _, dir := range []string{worktree, nested} {
		runGitIn(t, root, "worktree", "add", "--detach", "-q", dir)
	}
	p := &pathDisplay{home: home, cwd: root, roots: make(map[string]bool)}
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, tc := range []struct {
			name, path, want, region string
			roots                    []string
		}{
			{"main", root + "/internal/tui/app.go", "~/s/atlas/i/t/app.go", "atlas", []string{"atlas"}},
			{"linked", worktree + "/internal/app.go", "~/w/atlas.fix/i/app.go", "atlas.fix", []string{"atlas.fix"}},
			{"nested", nested + "/app.go", "~/s/atlas/.t/feature/app.go", "atlas", []string{"atlas", "feature"}},
			{"relative", "internal/界é/app.go", "i/界/app.go", "internal", nil},
			{"combining", "éxample/app.go", "é/app.go", "éxample", nil},
			{"outside", home + "/notes/app.go", "~/n/app.go", "", nil},
			{"home boundary", home + "-other/app.go", "", "", nil},
			{"leave repository", "../atlas-copy/app.go", "../a/app.go", "", nil},
		} {
			t.Run(fmt.Sprintf("dark=%t/%s", dark, tc.name), func(t *testing.T) {
				tab := p.render(tc.path, activeTabStyle, true, true)
				if plain := ansi.Strip(tab); tc.want != "" && plain != tc.want {
					t.Fatalf("path = %q, want %q", plain, tc.want)
				}
				tabRunes, tabStyles := renderedCells(tab)
				for i, style := range tabStyles {
					if !sameColor(style.Bg, activeTabStyle.GetBackground()) {
						t.Fatal("tab path changes the background")
					}
					if i < len(tabRunes)-len([]rune("app.go")) && tabRunes[i] != '/' && !sameColor(style.Fg, lipgloss.LightDark(dark)(lipgloss.Color("#00535F"), lipgloss.Color("#73B8C5"))) {
						t.Fatal("tab directory does not use the scheme's cyan")
					}
				}
				base := headerPathStyle
				rendered := p.render(tc.path, base, false, true)
				plain := ansi.Strip(rendered)
				if tc.name == "home boundary" && strings.Contains(plain, "~") {
					t.Fatalf("unrelated home prefix collapsed: %q", plain)
				}
				runes, styles := renderedCells(rendered)
				start := -1
				if tc.region != "" {
					start = len([]rune(plain[:strings.Index(plain, tc.region)]))
				}
				background := lipgloss.Color("#D2BDDF")
				filename := len(runes) - len([]rune("app.go"))
				for i, r := range runes {
					wantBG := base.GetBackground()
					if start >= 0 && i >= start {
						wantBG = background
					}
					if !sameColor(styles[i].Bg, wantBG) {
						t.Fatalf("background at %d (%q) = %v, want %v", i, r, styles[i].Bg, wantBG)
					}
					if i < filename && r != '/' && !sameColor(styles[i].Fg, lipgloss.Color("#00758F")) {
						t.Fatalf("header directory %q does not use blue cyan", r)
					}
					if r == '/' && sameColor(styles[i].Fg, lipgloss.Cyan) {
						t.Fatal("slash uses the directory color")
					}
				}
				for _, root := range tc.roots {
					at := len([]rune(plain[:strings.Index(plain, root)]))
					if styles[at].Attrs&uv.AttrBold == 0 || styles[at].Attrs&uv.AttrItalic != 0 {
						t.Fatalf("root %q is not emphasized in full", root)
					}
				}
			})
		}
	}
}

func TestPathColorsSurviveHeaderAndTabClipping(t *testing.T) {
	wasDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(wasDark) })
	app := setupAppWithDoc(t, "one\ntwo\n")
	root := filepath.Dir(app.filePath)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	app.paths = &pathDisplay{home: filepath.Dir(root), cwd: root, roots: make(map[string]bool)}
	app.width = 100
	for _, dark := range []bool{true, false} {
		initAdaptiveStyles(dark)
		for _, width := range []int{100, 24} {
			app.width = width
			header := strings.Split(app.renderHeader(), "\n")[1]
			plain := ansi.Strip(header)
			if !strings.Contains(plain, ".md L0/3") || strings.Contains(plain, ".md  ") {
				t.Fatalf("header lost filename or added space: %q", plain)
			}
			_, headerStyles := renderedCells(header)
			position := len([]rune(plain[:strings.Index(plain, "L0/3")]))
			for _, offset := range []int{0, 1, 3} {
				if !sameColor(headerStyles[position+offset].Fg, lipgloss.Color("#00758F")) {
					t.Fatal("header position does not match directory text")
				}
			}
			slash := headerStyles[position+2]
			if !sameColor(slash.Fg, lipgloss.Color("#81608E")) || slash.Attrs&uv.AttrBold == 0 {
				t.Fatal("header position slash is not muted and bold")
			}
			progress := strings.LastIndex(plain, "0/0")
			if progress < 0 {
				t.Fatal("header progress control disappeared")
			}
			progressCell := len([]rune(plain[:progress]))
			if !sameColor(headerStyles[progressCell].Bg, lipgloss.LightDark(dark)(lipgloss.Color("#E8E8E8"), lipgloss.Color("#3a3a3a"))) {
				t.Fatal("header progress control lost its scheme's background")
			}
			tab := app.renderTabWidth(app.tabLabels(), 0, width)
			for name, rendered := range map[string]string{"header": header, "tab": tab} {
				if lipgloss.Width(rendered) > width {
					t.Fatalf("%s exceeds width %d: %q", name, width, rendered)
				}
				runes, styles := renderedCells(rendered)
				if name == "tab" {
					for _, style := range styles {
						if !sameColor(style.Bg, activeTabStyle.GetBackground()) {
							t.Fatal("clipped tab path changes the background")
						}
					}
					continue
				}
				last := -1
				for i, style := range styles {
					if sameColor(style.Bg, lipgloss.Color("#D2BDDF")) {
						last = i
					}
				}
				if last < 0 || runes[last] == ' ' {
					t.Fatalf("%s root background includes a trailing space or disappeared: %q", name, rendered)
				}
			}
		}
	}
}
