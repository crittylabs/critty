package tui

import (
	"regexp"
	"strings"
	"testing"

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
		re, err := compileISearch(tt.query)
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
	if !app.isearch.failing || !strings.Contains(ansi.Strip(app.renderFooter()), "Failing I-search: alpha") {
		t.Fatalf("stepping past the last match did not fail: %q", ansi.Strip(app.renderFooter()))
	}
	app = ctrlKey(app, 's')
	assertCursor(t, app, 1)
	if prompt := ansi.Strip(app.renderFooter()); !strings.Contains(prompt, "Wrapped I-search: alpha") || !strings.Contains(prompt, "(1/3)") {
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
	if !strings.Contains(ansi.Strip(app.renderFooter()), "I-search backward: alp") {
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
	if prompt := ansi.Strip(app.renderFooter()); !strings.Contains(prompt, "[incomplete regexp]") {
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
	out, ok := highlightISearch(styled, regexp.MustCompile("bar|baz"), 1)
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
