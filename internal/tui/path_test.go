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

	"github.com/crittylabs/critty/internal/review"
)

func TestProjectTreePaths(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "project")
	worktree := filepath.Join(parent, "worktree")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(worktree, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../metadata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cwd, path, want string
	}{
		{"absolute", repo, filepath.Join(repo, "internal", "a.go"), "internal/a.go"},
		{"subdirectory", filepath.Join(repo, "internal"), filepath.Join(repo, "internal", "a.go"), "internal/a.go"},
		{"relative", repo, "internal/a.go", "internal/a.go"},
		{"linked worktree", worktree, filepath.Join(worktree, "a.go"), "a.go"},
		{"non-git", parent, filepath.Join(parent, "notes", "a.md"), "notes/a.md"},
		{"outside", repo, filepath.Join(parent, "a.go"), filepath.Join(parent, "a.go")},
		{"sibling prefix", repo, repo + "-other/a.go", repo + "-other/a.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pathDisplay{cwd: tc.cwd, roots: make(map[string]bool)}
			if got := p.treePath(tc.path); got != tc.want {
				t.Fatalf("tree path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTabLabelsKeepOnlyNecessaryRootPrefixes(t *testing.T) {
	parent := t.TempDir()
	first, second := filepath.Join(parent, "alpha", "project"), filepath.Join(parent, "alpine", "project")
	worktree := filepath.Join(parent, "project.fix")
	for _, root := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(worktree, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../metadata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := newFileTreeTestApp()
	app.paths = &pathDisplay{cwd: first, roots: make(map[string]bool)}
	source := filepath.Join(first, "internal", "tui", "a.go")
	app.tabs = []FileTab{
		{path: source},
		{path: filepath.Join(first, "README.md")},
		{path: overallCommentsPath},
		{path: "attachments/check.md", attachment: &review.AttachedDocument{Name: "check.md", Source: filepath.Join(second, "check.md")}},
	}
	assertLabel := func(index int, want string) {
		t.Helper()
		if got := ansi.Strip(app.tabLabels()[index].text); got != want {
			t.Fatalf("tab %d = %q, want %q", index, got, want)
		}
	}
	assertLabel(0, "i/t/a.go")
	assertLabel(1, "README.md")
	app.tabs = append(app.tabs, FileTab{path: filepath.Join(second, "internal", "tui", "a.go")})
	assertLabel(0, "alpha/project/i/t/a.go")
	assertLabel(4, "alpine/project/i/t/a.go")
	app.tabs = append(app.tabs, FileTab{path: filepath.Join(worktree, "a.go")})
	assertLabel(5, "project.fix/a.go")
	app.tabs = app.tabs[:4]
	assertLabel(0, "i/t/a.go")
	if app.tabs[0].path != source {
		t.Fatal("tab labels changed the source path")
	}
}

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
			header := strings.Split(app.renderHeader(), "\n")[lipgloss.Height(app.renderReviewBar())]
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

func TestHeaderPathWeightStartsAtProjectRoot(t *testing.T) {
	for _, tc := range []struct {
		name, path, prefix string
		home               string
		git                bool
	}{
		{name: "absolute", path: "/prefix/proj/reldir/file.ext", prefix: "/prefix/", git: true},
		{name: "home", path: "/prefix/proj/reldir/file.ext", prefix: "~/", home: "/prefix", git: true},
		{name: "relative", path: "reldir/file.ext", git: true},
		{name: "outside", path: "/outside/file.ext", prefix: "/outside/", git: true},
		{name: "non-git", path: "/prefix/proj/reldir/file.ext", prefix: "/prefix/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pathDisplay{cwd: "/prefix/proj", home: tc.home, roots: map[string]bool{"/prefix/proj": tc.git}}
			path := p.render(tc.path, headerPathStyle, false, true)
			rendered := headerStyle.Render(inlineBackground(headerPathStyle, headerPathStyle.Render(path)))
			runes, styles := renderedCells(rendered)
			plain := string(runes)
			start := strings.Index(plain, ansi.Strip(path))
			if start < 0 {
				t.Fatalf("missing path: %q", plain)
			}
			for i, r := range []rune(ansi.Strip(path)) {
				want := i >= len([]rune(tc.prefix))
				if got := styles[start+i].Attrs&uv.AttrBold != 0; got != want {
					t.Fatalf("character %d (%c) bold=%t, want %t: %q", i, r, got, want, plain)
				}
			}
		})
	}
}
