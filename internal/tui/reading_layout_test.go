package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReadingLayout(t *testing.T) {
	const source = "before **bold** " + wrapWords + " end"
	for _, tc := range []struct {
		name                     string
		readOnly                 bool
		numbers                  *bool
		cap, terminal            int
		wantGutter, wantViewport int
	}{
		{name: "view full width", readOnly: true, terminal: 100, wantGutter: 1, wantViewport: 98},
		{name: "review default", terminal: 100, wantGutter: 7, wantViewport: 98},
		{name: "view override", readOnly: true, numbers: new(true), terminal: 100, wantGutter: 7, wantViewport: 98},
		{name: "review override", numbers: new(false), terminal: 100, wantGutter: 1, wantViewport: 98},
		{name: "limited", readOnly: true, cap: 30, terminal: 100, wantGutter: 1, wantViewport: 32},
		{name: "narrow terminal", readOnly: true, cap: 80, terminal: 40, wantGutter: 1, wantViewport: 38},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sample.md")
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			app := NewApp(path, AppConfig{ReadOnly: tc.readOnly, LineNumbers: tc.numbers, MaxWrapWidth: tc.cap})
			app, _ = updateApp(app, docRenderedMsg{})
			app.viewMode = true
			app, _ = updateApp(app, tea.WindowSizeMsg{Width: tc.terminal, Height: 30})
			if app.sourceGutterWidth() != tc.wantGutter || app.contentViewport.Width() != tc.wantViewport {
				t.Fatalf("gutter=%d viewport=%d", app.sourceGutterWidth(), app.contentViewport.Width())
			}
			for range 2 {
				spans := app.contentLayout.textSpans
				first, last := spans[0], spans[len(spans)-1]
				left, top, _, _ := app.contentBounds()
				app = pressMouse(app, left+first.left, top+first.row)
				app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
				if got := app.selectedText(); got != source {
					t.Fatalf("copy=%q", got)
				}
				for _, row := range strings.Split(app.View().Content, "\n")[top : top+app.contentViewport.Height()] {
					if ansi.StringWidth(row) != tc.terminal || ansi.Strip(ansi.Cut(row, tc.terminal-1, tc.terminal)) != "│" {
						t.Fatalf("frame shrank or overflowed: %q", row)
					}
				}
				before := app.hideLineNumbers
				app = pressKey(app, '#')
				if app.hideLineNumbers == before || app.tab().cursorLine != 1 {
					t.Fatal("toggle lost source location")
				}
			}
		})
	}
}

func TestHiddenDiffLineNumbers(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			app := patchApp(t, formatsPatch)
			app.split = split
			app.rebuildContent()
			app = pressKey(app, '#')
			if app.sourceGutterWidth() != 1 {
				t.Fatal("gutter did not shrink")
			}
			for _, span := range app.contentLayout.textSpans {
				if strings.Contains(span.text, "old one") || strings.Contains(span.text, "new one") {
					left, top, _, _ := app.contentBounds()
					app = pressMouse(app, left+span.left, top+span.row)
					app = releaseMouse(app, left+span.left+ansi.StringWidth(span.text), top+span.row)
					if app.selectedText() != span.text {
						t.Fatalf("side %s copy=%q want=%q", span.ref.side, app.selectedText(), span.text)
					}
				}
			}
		})
	}
}

func TestLineNumbersMouseAndReadingPosition(t *testing.T) {
	app := newViewNavigationApp(t, strings.Repeat(wrapWords+"\n", 40))
	app.tab().cursorLine = 20
	app.rebuildContent()
	app.scrollToCursor()
	target := contentMouseTarget{line: 20}
	before, ok := app.diffAnchorRow(target)
	if !ok {
		t.Fatal("missing source line")
	}
	before -= app.contentViewport.YOffset()
	_, layout := app.renderReviewScreen()
	if !layout.hasLineNumbersButton {
		t.Fatal("line number button unavailable")
	}
	app = clickMouse(app, layout.lineNumbersButton.left, layout.lineNumbersButton.top)
	after, _ := app.diffAnchorRow(target)
	if app.hideLineNumbers || app.tab().cursorLine != 20 || after-app.contentViewport.YOffset() != before {
		t.Fatal("toggle moved reading position")
	}
}
