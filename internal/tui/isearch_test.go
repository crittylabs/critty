package tui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const isearchDoc = "alpha\nbeta\nalphabet\ngamma\nalpha\n"

func ctrlKey(app AppModel, code rune) AppModel {
	app, _ = updateApp(app, tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
	return app
}

func typeSearch(app AppModel, text string) AppModel {
	for _, r := range text {
		app, _ = updateApp(app, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return app
}

func newISearchApp(t *testing.T, cursorLine int) AppModel {
	t.Helper()
	app := setupAppWithDoc(t, isearchDoc)
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 30})
	app.tab().cursorLine = cursorLine
	return app
}

func assertCursor(t *testing.T, app AppModel, want int) {
	t.Helper()
	if got := app.tab().cursorLine; got != want {
		t.Fatalf("cursor line = %d, want %d", got, want)
	}
}

func TestISearchSmartCase(t *testing.T) {
	for _, tt := range []struct {
		query, text string
		want        bool
	}{
		{"alpha", "ALPHA", true},
		{"Alpha", "alpha", false},
		{"Alpha", "Alpha", true},
		{`\w+`, "ALPHA", true},
		{`\W`, "x", false},
	} {
		re, err := isearchOptions{}.compile(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := re.MatchString(tt.text); got != tt.want {
			t.Errorf("%q matches %q = %v, want %v", tt.query, tt.text, got, tt.want)
		}
	}
}

func TestISearchEditsSearchAgainFromOrigin(t *testing.T) {
	app := newISearchApp(t, 2)
	app = ctrlKey(app, 's')
	if !app.isearch.active {
		t.Fatal("ctrl+s did not start the search")
	}
	app = typeSearch(app, "a")
	assertCursor(t, app, 2) // "beta" on the origin line
	app = typeSearch(app, "l")
	assertCursor(t, app, 3)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	assertCursor(t, app, 2)
	if app.isearch.query != "a" {
		t.Fatalf("query = %q, want %q", app.isearch.query, "a")
	}
}

func TestISearchStepsFailsAndWraps(t *testing.T) {
	app := newISearchApp(t, 2)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "alpha")
	assertCursor(t, app, 3)
	app = ctrlKey(app, 's')
	assertCursor(t, app, 5)
	app = ctrlKey(app, 's')
	assertCursor(t, app, 5)
	if !app.isearch.failing || !strings.Contains(ansi.Strip(app.renderFooter()), "Failing regexp I-search: alpha") {
		t.Fatalf("stepping past the last match did not fail: %q", ansi.Strip(app.renderFooter()))
	}
	app = ctrlKey(app, 's')
	assertCursor(t, app, 1)
	if prompt := ansi.Strip(app.renderFooter()); !strings.Contains(prompt, "Wrapped regexp I-search: alpha") || !strings.Contains(prompt, "(1/3)") {
		t.Fatalf("wrapped prompt = %q", ansi.Strip(app.renderFooter()))
	}
	app = ctrlKey(app, 'r')
	assertCursor(t, app, 1)
	if !app.isearch.failing {
		t.Fatal("stepping back from the first match did not fail")
	}
	app = ctrlKey(app, 'r')
	assertCursor(t, app, 5)
	app = ctrlKey(app, 'r')
	assertCursor(t, app, 3)
}

func TestISearchBackwardIncludesOriginLine(t *testing.T) {
	app := newISearchApp(t, 4)
	app = ctrlKey(app, 'r')
	app = typeSearch(app, "a")
	assertCursor(t, app, 4) // "gamma" on the origin line
	app = typeSearch(app, "lp")
	assertCursor(t, app, 3)
	if !strings.Contains(ansi.Strip(app.renderFooter()), "Regexp I-search backward: alp") {
		t.Fatalf("prompt = %q", ansi.Strip(app.renderFooter()))
	}
}

func TestISearchExitKeepsOrRestoresPosition(t *testing.T) {
	app := newISearchApp(t, 2)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "gamma")
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.isearch.active {
		t.Fatal("enter did not leave the search")
	}
	assertCursor(t, app, 4)

	app = ctrlKey(app, 's')
	app = typeSearch(app, "alpha")
	assertCursor(t, app, 5)
	app = ctrlKey(app, 'g')
	if app.isearch.active {
		t.Fatal("ctrl+g did not leave the search")
	}
	assertCursor(t, app, 4)

	app = ctrlKey(app, 'r')
	app = ctrlKey(app, 'r')
	if app.isearch.query != "alpha" {
		t.Fatalf("repeated ctrl+r resumed %q, want the last query", app.isearch.query)
	}
	assertCursor(t, app, 3)
	app = ctrlKey(app, 'c')
	if app.isearch.active {
		t.Fatal("ctrl+c did not leave the search")
	}
	assertCursor(t, app, 4)

	app = ctrlKey(app, 'r')
	app = typeSearch(app, "beta")
	assertCursor(t, app, 2)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.isearch.active {
		t.Fatal("esc did not leave the search")
	}
	assertCursor(t, app, 4)
}

func TestISearchOtherKeyLeavesAndActs(t *testing.T) {
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "gamma")
	app = pressKey(app, tea.KeyDown)
	if app.isearch.active {
		t.Fatal("down did not leave the search")
	}
	assertCursor(t, app, 5)
}

func TestISearchInvalidRegexpKeepsPosition(t *testing.T) {
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "beta(")
	assertCursor(t, app, 2)
	if prompt := ansi.Strip(app.renderFooter()); !strings.Contains(prompt, "[incomplete input]") {
		t.Fatalf("prompt = %q", prompt)
	}
	app = typeSearch(app, ")")
	assertCursor(t, app, 2)
}

func TestISearchHighlightsMatches(t *testing.T) {
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	app = typeSearch(app, "alpha")
	content := app.contentViewport.View()
	if !strings.Contains(content, isearchCurrentStyle.Render("alpha")) {
		t.Fatalf("current match is not highlighted: %q", content)
	}
	if strings.Count(content, isearchMatchStyle.Render("alpha")) != 2 {
		t.Fatal("other matches are not highlighted")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if content := app.contentViewport.View(); strings.Contains(content, isearchMatchStyle.Render("alpha")) {
		t.Fatal("highlights remain after leaving the search")
	}
}

func TestHighlightISearchKeepsSurroundingStyles(t *testing.T) {
	styled := "\x1b[31mfoo bar baz\x1b[0m"
	out, ok := highlightISearch(styled, findNonEmpty(regexp.MustCompile("bar|baz"), ansi.Strip(styled)), 1)
	if !ok {
		t.Fatal("no match reported")
	}
	if ansi.Strip(out) != "foo bar baz" {
		t.Fatalf("text changed: %q", ansi.Strip(out))
	}
	if !strings.Contains(out, isearchMatchStyle.Render("bar")) || !strings.Contains(out, isearchCurrentStyle.Render("baz")) {
		t.Fatalf("matches are not styled: %q", out)
	}
	if !strings.HasPrefix(out, "\x1b[31mfoo ") {
		t.Fatalf("leading style lost: %q", out)
	}
}

func TestISearchSlashAndQuestionMarkStartSearches(t *testing.T) {
	app := newISearchApp(t, 3)
	app = pressKey(app, '/')
	if !app.isearch.active || !app.isearch.forward {
		t.Fatal("/ did not start a forward search")
	}
	app = typeSearch(app, "alpha")
	assertCursor(t, app, 3)
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})

	app = pressKey(app, '?')
	if !app.isearch.active || app.isearch.forward {
		t.Fatal("? did not start a backward search")
	}
	if app.modal == helpModal {
		t.Fatal("? opened help")
	}
	app = ctrlKey(app, 'r') // an empty query resumes the last one
	assertCursor(t, app, 3)
	app = typeSearch(app, "/?")
	if app.isearch.query != "alpha/?" {
		t.Fatalf("/ and ? in a search were not typed: %q", app.isearch.query)
	}
}

func TestISearchToggles(t *testing.T) {
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	prompt := func() string { return ansi.Strip(app.renderFooter()) }

	app = pressAltKey(app, 's')
	if !strings.Contains(prompt(), "[M-s:") {
		t.Fatalf("M-s did not show its menu: %q", prompt())
	}
	app = typeSearch(app, "r")
	if app.isearch.mode != isearchLiteral || app.isearch.query != "" || !strings.HasPrefix(prompt(), "I-search: ") {
		t.Fatalf("M-s r did not turn off regexp mode: %q", prompt())
	}
	app = typeSearch(app, "a.p")
	if app.isearch.found {
		t.Fatal("literal search matched a regexp")
	}

	app = pressAltKey(app, 's')
	app = typeSearch(app, "w")
	if app.isearch.mode != isearchWord || !strings.HasPrefix(prompt(), "Failing word I-search: a.p") {
		t.Fatalf("M-s w did not turn on word mode: %q", prompt())
	}
	app = pressAltKey(app, 's')
	app = typeSearch(app, "r")
	if app.isearch.mode != isearchRegexp || !app.isearch.found || !strings.HasPrefix(prompt(), "Regexp I-search: a.p") {
		t.Fatalf("M-s r did not switch word mode to regexp mode: %q", prompt())
	}

	app = typeSearch(app, "H")
	if !strings.HasPrefix(prompt(), "Failing case-sensitive regexp I-search: a.pH") {
		t.Fatalf("an uppercase letter did not make the search case sensitive: %q", prompt())
	}
	app = pressAltKey(app, 's')
	app = typeSearch(app, "c")
	if !app.isearch.found || !strings.HasPrefix(prompt(), "Case-insensitive regexp I-search: a.pH") {
		t.Fatalf("M-s c did not fold case: %q", prompt())
	}

	app = ctrlKey(app, 'g')
	app = ctrlKey(app, 's')
	if !app.isearch.caseFold || app.isearch.mode != isearchRegexp {
		t.Fatal("search options did not persist to the next search")
	}
}

func TestISearchWordMode(t *testing.T) {
	for _, tt := range []struct {
		query, text string
		want        bool
	}{
		{"alpha", "alphabet", true}, // the last word may be incomplete
		{"alpha ", "alphabet", false},
		{"alpha ", "alpha bet", true},
		{"foo bar", "foo, bar", true},
		{"foo bar", "xfoo bar", false},
		{"a.b", "a-b", true},
		{"!!", "!!", false},
		{"検索", "検索する", true},
		{"検索 ", "検索する", false},
		{"検索 ", "検索、する", true},
		{"検索", "全文検索", false},
		{"検索 語", "検索・語", true},
		{"検索 語", "検索別語", false},
		{"cafe\u0301 ", "cafe\u0301!", true},
		{"cafe ", "cafe\u0301!", false},
		{"é ", "é!", true},
		{"foo ", "éfoo!", false},
		{"foo bar", "xfoo bar foo bar", true},
		{"foo foo", "xfoo foo foo", true},
	} {
		re, err := isearchOptions{mode: isearchWord}.compile(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		s := isearchState{isearchOptions: isearchOptions{mode: isearchWord}, query: tt.query, re: re}
		if got := len(s.findMatches(tt.text)) > 0; got != tt.want {
			t.Errorf("word %q matches %q = %v, want %v", tt.query, tt.text, got, tt.want)
		}
	}
}

func TestISearchUnicodeWordHighlights(t *testing.T) {
	app := setupAppWithDoc(t, "全文検索\n検索、検索する\n")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 30})
	app.tab().cursorLine = 1
	app.isearch.mode = isearchWord
	app = ctrlKey(app, 's')
	app = typeSearch(app, "検索 ")
	if app.isearch.count != 1 || app.isearch.match.start != 0 || app.isearch.match.end != len("検索") {
		t.Fatalf("count=%d match=%+v", app.isearch.count, app.isearch.match)
	}
	assertCursor(t, app, 2)
	decorate := app.isearchDecorator(lineRef{line: 2})
	text, found := decorate("検索、検索する")
	if !found || !strings.Contains(text, isearchCurrentStyle.Render("検索")) || strings.Contains(text, isearchMatchStyle.Render("検索")) {
		t.Fatalf("word highlights disagree with matches: %q", text)
	}
}

func TestISearchCapturesFollowingText(t *testing.T) {
	app := setupAppWithDoc(t, "intro\nsee foo.Bar_baz qux\nfoo.bar_baz\n検索する機能\n")
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 30})
	app.tab().cursorLine = 1
	app = ctrlKey(app, 's')
	app = ctrlKey(app, 'd') // no current match: ignored
	app = ctrlKey(app, 'w')
	if app.isearch.query != "" {
		t.Fatalf("capture without a match changed the query to %q", app.isearch.query)
	}

	app = typeSearch(app, "foo")
	assertCursor(t, app, 2)
	app = ctrlKey(app, 'd')
	if app.isearch.query != `foo\.` {
		t.Fatalf("ctrl+d captured %q, want an escaped dot", app.isearch.query)
	}
	app = ctrlKey(app, 'w')
	if app.isearch.query != `foo\.bar_baz` {
		t.Fatalf("ctrl+w captured %q, want the lowercased word", app.isearch.query)
	}
	assertCursor(t, app, 2) // still on the match the capture extended
	app = ctrlKey(app, 'w')
	if app.isearch.query != `foo\.bar_baz ` {
		t.Fatalf("ctrl+w on a space captured %q, want one character", app.isearch.query)
	}

	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if app.isearch.query != `foo\.` {
		t.Fatalf("backspace removed %q, want whole captured units", app.isearch.query)
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if app.isearch.query != "fo" {
		t.Fatalf("backspace on typed text left %q, want one character removed", app.isearch.query)
	}

	app = ctrlKey(app, 'g')
	app = ctrlKey(app, 's')
	app = typeSearch(app, "検")
	app = ctrlKey(app, 'w')
	app = ctrlKey(app, 'w')
	if app.isearch.query != "検索す" {
		t.Fatalf("ctrl+w on kanji and kana captured %q, want one character each", app.isearch.query)
	}
}

func TestNextWord(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"bar_baz qux", "bar_baz"},
		{"Ünïcode9 x", "Ünïcode9"},
		{" x", " "},
		{".x", "."},
		{"機能", "機"},
		{"カタカナ", "カ"},
	} {
		if got := nextWord(tt.in); got != tt.want {
			t.Errorf("nextWord(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func assertTabCursor(t *testing.T, app AppModel, tab, line int) {
	t.Helper()
	if app.activeTab != tab || app.tab().cursorLine != line {
		t.Fatalf("at tab %d line %d, want tab %d line %d", app.activeTab, app.tab().cursorLine, tab, line)
	}
}

func TestISearchAcrossFiles(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.width = 120
	prompt := func() string { return ansi.Strip(app.renderFooter()) }

	app = ctrlKey(app, 's')
	app = pressAltKey(app, 's')
	app = typeSearch(app, "m")
	app = typeSearch(app, "three")
	assertTabCursor(t, app, 0, 3)
	if p := prompt(); !strings.HasPrefix(p, "Regexp multi-file I-search: three") || !strings.Contains(p, "(1/3, file 1/3)") {
		t.Fatalf("prompt = %q", p)
	}
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 1, 3)
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 2, 3)
	if p := prompt(); !strings.Contains(p, "(3/3, file 3/3)") {
		t.Fatalf("prompt = %q", p)
	}
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 2, 3)
	if !app.isearch.failing {
		t.Fatal("stepping past the last file did not fail")
	}
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 0, 3)
	app = ctrlKey(app, 'r')
	app = ctrlKey(app, 'r')
	assertTabCursor(t, app, 2, 3)
	app = ctrlKey(app, 'r')
	assertTabCursor(t, app, 1, 3)

	// Turning the mode off searches the origin file again; backward from its
	// first line, the search fails there.
	app = pressAltKey(app, 's')
	app = typeSearch(app, "m")
	assertTabCursor(t, app, 0, 1)
	if p := prompt(); !strings.HasPrefix(p, "Failing regexp I-search backward: three") || !strings.Contains(p, "(0/1)") {
		t.Fatalf("prompt = %q", p)
	}
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 0, 3)

	app = pressAltKey(app, 's')
	app = typeSearch(app, "m")
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 1, 3)
	app = ctrlKey(app, 'g')
	assertTabCursor(t, app, 0, 1)
	if !app.isearch.allFiles {
		t.Fatal("multi-file mode did not persist")
	}
}

func TestISearchAcrossFilesStartsInLaterFile(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.isearch.allFiles = true
	app.tab().cursorLine = 4
	app = ctrlKey(app, 's')
	app = typeSearch(app, "two")
	assertTabCursor(t, app, 1, 2)
	app = ctrlKey(app, 'g')
	app = ctrlKey(app, 'r')
	app = typeSearch(app, "four")
	assertTabCursor(t, app, 0, 4) // the origin line itself
}

func TestISearchSelectionStaysInFile(t *testing.T) {
	app := newCommentNavigationTestApp()
	app.width = 120
	app.isearch.allFiles = true
	app = pressKey(app, 'v')
	app = ctrlKey(app, 's')
	app = typeSearch(app, "four")
	assertTabCursor(t, app, 0, 4)
	app = ctrlKey(app, 's')
	assertTabCursor(t, app, 0, 4)
	if p := ansi.Strip(app.renderFooter()); strings.Contains(p, "multi-file") {
		t.Fatalf("a selection searched other files: %q", p)
	}
}

func TestISearchPasteAndYank(t *testing.T) {
	for _, source := range []string{"terminal", "kill-ring"} {
		for _, tt := range []struct {
			name, input, want string
		}{
			{"Japanese", "検索🙂", "検索🙂"},
			{"regexp", "alpha|beta", "alpha|beta"},
			{"multiline", "one\r\ntwo\tthree\rfour", "one two three four"},
			{"controls", "\x1b[31malpha\x1b[0m\x00", "alpha"},
			{"empty", "", ""},
		} {
			t.Run(source+"/"+tt.name, func(t *testing.T) {
				app := newISearchApp(t, 1)
				app = ctrlKey(app, 's')
				app = typeSearch(app, "a")
				if source == "terminal" {
					app, _ = updateApp(app, tea.PasteMsg{Content: tt.input})
				} else {
					if tt.input != "" {
						app.killRing.entries = []string{tt.input, "older"}
					}
					app = ctrlKey(app, 'y')
				}
				if !app.isearch.active || app.isearch.query != "a"+tt.want {
					t.Fatalf("active=%v query=%q, want %q", app.isearch.active, app.isearch.query, "a"+tt.want)
				}
				if tt.want != "" {
					app = ctrlKey(app, 'h')
					if app.isearch.query != "a" {
						t.Fatalf("backspace left %q, want the query before paste", app.isearch.query)
					}
				}
			})
		}
	}
	app := newISearchApp(t, 1)
	app = ctrlKey(app, 's')
	app, _ = updateApp(app, tea.PasteMsg{Content: "beta"})
	assertCursor(t, app, 2)
	app = ctrlKey(app, 'g')
	assertCursor(t, app, 1)
	if app.View().Cursor != nil {
		t.Fatal("terminal cursor remains after cancelling search")
	}
}

func TestISearchTerminalCursor(t *testing.T) {
	for _, query := range []string{"", "検索", "e\u0301🙂", strings.Repeat("日本語", 20)} {
		for _, width := range []int{18, 40, 100} {
			app := newISearchApp(t, 1)
			app = ctrlKey(app, 's')
			app = typeSearch(app, query)
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: width, Height: 30})
			view := app.View()
			cursor := view.Cursor
			if cursor == nil || cursor.X < 0 || cursor.X >= width || cursor.Y < 0 || cursor.Y >= 30 {
				t.Fatalf("query=%q width=%d cursor=%+v", query, width, cursor)
			}
			rows := strings.Split(ansi.Strip(view.Content), "\n")
			prompt := rows[cursor.Y]
			if cell := ansi.Cut(prompt, cursor.X, cursor.X+1); cell != " " {
				t.Fatalf("cursor at %d,%d covers %q in %q", cursor.X, cursor.Y, cell, prompt)
			}
			if query != "" {
				_, n := utf8.DecodeLastRuneInString(query)
				if before := ansi.Cut(prompt, 0, cursor.X); !strings.HasSuffix(before, query[len(query)-n:]) {
					t.Fatalf("input end missing before cursor: %q", before)
				}
			}
		}
	}
}

func TestISearchYanksCommentEditorKill(t *testing.T) {
	app := newISearchApp(t, 1)
	app = pressKey(app, tea.KeyEnter)
	if app.modal != commentModal {
		t.Fatalf("modal=%v, want comment editor", app.modal)
	}
	app, _ = updateApp(app, tea.PasteMsg{Content: "beta"})
	app = ctrlKey(app, 'a')
	app = ctrlKey(app, 'k')
	app = pressKey(app, tea.KeyEscape)
	if app.modal != noModal {
		t.Fatalf("modal=%v after cancelling empty comment", app.modal)
	}
	app = ctrlKey(app, 's')
	app = ctrlKey(app, 'y')
	if app.isearch.query != "beta" || !app.isearch.active {
		t.Fatalf("query=%q active=%v", app.isearch.query, app.isearch.active)
	}
	assertCursor(t, app, 2)
	app = ctrlKey(app, 'y')
	if app.isearch.query != "betabeta" || len(app.killRing.entries) != 1 || app.killRing.entries[0] != "beta" {
		t.Fatalf("query=%q ring=%v after repeated yank", app.isearch.query, app.killRing.entries)
	}
}
