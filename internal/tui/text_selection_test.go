package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDragTextAndCopy(t *testing.T) {
	for _, tt := range []struct {
		name, content, want string
		start, end          int
		reverse             bool
	}{
		{name: "inline code", content: "before `word` after", start: 8, end: 12, want: "`word`"},
		{name: "reverse", content: "before `word` after", start: 8, end: 12, want: "`word`", reverse: true},
		{name: "partial link", content: "before [long label](https://example.org/a_(b)) after", start: 7, end: 11, want: "[long](https://example.org/a_(b))"},
		{name: "unicode", content: "`日本語 é 👩‍💻`", start: 1, end: 12, want: "`日本語 é 👩‍💻`"},
		{name: "inside wide character", content: "`日本語`", start: 2, end: 5, want: "`日本`"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := setupAppWithDoc(t, tt.content+"\n")
			app.width, app.height = 100, 24
			app.recalculateLayout()
			app.rebuildContent()
			left, top, _, _ := app.contentBounds()
			x, y := left+gutterWidth, top+app.contentLayout.textSpans[0].row
			start, end := x+tt.start, x+tt.end
			if tt.reverse {
				start, end = end, start
			}
			app = pressMouse(app, start, y)
			app = moveMouse(app, end, y)
			app = releaseMouse(app, end, y)
			if app.modal != noModal || app.tab().selecting || app.textSelection.dragging {
				t.Fatal("text drag entered line selection or opened a dialog")
			}
			if got := app.selectedText(); got != tt.want {
				t.Fatalf("selected = %q, want %q", got, tt.want)
			}
			before := ansi.Strip(app.contentViewport.GetContent())
			app, cmd := updateApp(app, tea.KeyPressMsg{Code: 'w', Mod: tea.ModAlt})
			if cmd == nil || !reflect.DeepEqual(cmd(), tea.SetClipboard(tt.want)()) {
				t.Fatal("M-w did not emit the selected text through OSC 52")
			}
			if app.killRing.current() != tt.want || app.textSelection.active {
				t.Fatalf("kill ring = %q, selection = %+v", app.killRing.current(), app.textSelection)
			}
			if ansi.Strip(app.contentViewport.GetContent()) != before {
				t.Fatal("selection decoration changed the rendered text")
			}
		})
	}
}

func TestTextSelectionWrappedRowsAndBlankLines(t *testing.T) {
	content := "one two three four five six seven eight nine ten eleven twelve\n\nlast line"
	app := setupAppWithDoc(t, content)
	app.width, app.height = 70, 32
	app.recalculateLayout()
	app.rebuildContent()
	spans := app.contentLayout.textSpans
	if len(spans) < 4 {
		t.Fatal("fixture did not wrap")
	}
	left, top, _, _ := app.contentBounds()
	first, last := spans[0], spans[len(spans)-1]
	app = pressMouse(app, left+first.left, top+first.row)
	app = moveMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
	app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text), top+last.row)
	if got := app.selectedText(); got != content {
		t.Fatalf("selected = %q, want %q", got, content)
	}
}

func TestTextSelectionCopyButtonCancelAndLayoutChange(t *testing.T) {
	for _, action := range []string{"button", "escape", "resize", "click", "scroll"} {
		t.Run(action, func(t *testing.T) {
			app := setupAppWithDoc(t, "`word` "+strings.Repeat("long text ", 10)+"\n")
			app.width, app.height = 100, 30
			app.recalculateLayout()
			app.rebuildContent()
			left, top, _, _ := app.contentBounds()
			app = pressMouse(app, left+gutterWidth+1, top)
			app = releaseMouse(app, left+gutterWidth+5, top)
			switch action {
			case "button":
				_, layout := app.renderReviewScreen()
				if !layout.hasCopyTextButton {
					t.Fatal("missing copy button")
				}
				var cmd tea.Cmd
				app, cmd = clickMouseCmd(app, layout.copyTextButton.left, layout.copyTextButton.top)
				if cmd == nil || !reflect.DeepEqual(cmd(), tea.SetClipboard("`word`")()) {
					t.Fatal("copy button did not copy text")
				}
			case "escape":
				app = pressKey(app, tea.KeyEscape)
			case "resize":
				app, _ = updateApp(app, tea.WindowSizeMsg{Width: 70, Height: 30})
			case "click":
				app = clickMouse(app, left+gutterWidth+1, top)
			case "scroll":
				app.contentViewport.ScrollDown(1)
				app.rebuildContent()
				if app.selectedText() != "`word`" {
					t.Fatal("scrolling lost selection")
				}
				return
			}
			if app.selectedText() != "" {
				t.Fatal("stale text selection survived")
			}
		})
	}
}

func TestTextSelectionDiffSides(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, old := range []bool{false, true} {
			t.Run(fmt.Sprintf("split=%v/old=%v", split, old), func(t *testing.T) {
				app := patchApp(t, formatsPatch)
				app.split = split
				app.rebuildContent()
				want := "new one"
				if old {
					want = "old one"
				}
				var span contentTextSpan
				found := false
				for _, candidate := range app.contentLayout.textSpans {
					if candidate.text == want {
						span, found = candidate, true
						break
					}
				}
				if !found {
					t.Fatalf("missing %q", want)
				}
				app.contentViewport.SetYOffset(span.row)
				left, top, _, _ := app.contentBounds()
				y := top + span.row - app.contentViewport.YOffset()
				app = pressMouse(app, left+span.left, y)
				end := left + span.left + ansi.StringWidth(want)
				app = releaseMouse(app, end, y)
				if got := app.selectedText(); got != want {
					t.Fatalf("selected = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestTextSelectionCopyButtonFitsShortTerminal(t *testing.T) {
	app := setupAppWithDoc(t, "`word`\n")
	app.width, app.height = 54, 18
	app.recalculateLayout()
	app.rebuildContent()
	left, top, _, _ := app.contentBounds()
	app = pressMouse(app, left+gutterWidth+1, top)
	app = releaseMouse(app, left+gutterWidth+5, top)
	_, layout := app.renderReviewScreen()
	if !layout.hasCopyTextButton || layout.copyTextButton.bottom > app.height {
		t.Fatalf("copy button is outside the terminal: %+v", layout.copyTextButton)
	}
	app = pressKey(app, tea.KeyEscape)
	if app.contentViewport.Height() != app.height-app.headerHeight()-app.tabBarHeight()-1 {
		t.Fatal("cancel did not restore the content height")
	}
}

func TestTextSelectionTableSource(t *testing.T) {
	for _, whole := range []bool{false, true} {
		t.Run(fmt.Sprintf("whole=%v", whole), func(t *testing.T) {
			content := "| small | other |\n| --- | --- |\n| x | much longer |"
			app := setupAppWithDoc(t, content)
			app.width, app.height = 100, 30
			app.recalculateLayout()
			app.rebuildContent()
			spans := app.contentLayout.textSpans
			first, last := spans[0], spans[0]
			start := strings.Index(first.text, "small")
			end := start + len("small")
			want := "small"
			if whole {
				last = spans[len(spans)-1]
				start, end, want = 0, len(last.text), content
			}
			left, top, _, _ := app.contentBounds()
			app = pressMouse(app, left+first.left+ansi.StringWidth(first.text[:start]), top+first.row)
			app = releaseMouse(app, left+last.left+ansi.StringWidth(last.text[:end]), top+last.row)
			if got := app.selectedText(); got != want {
				t.Fatalf("copied = %q, want %q", got, want)
			}
		})
	}
}

func TestTextSelectionSplitUsesBeforeMarkdownContext(t *testing.T) {
	patch := "diff --git a/f.md b/f.md\n--- a/f.md\n+++ b/f.md\n@@ -1,5 +1,5 @@\n-```\n+text\n `code`\n-```\n+text\n \n tail\n"
	app := patchApp(t, patch)
	app.split = true
	app.rebuildContent()
	var span contentTextSpan
	for _, candidate := range app.contentLayout.textSpans {
		if candidate.column == 0 && candidate.text == "`code`" {
			span = candidate
			break
		}
	}
	if span.text == "" {
		t.Fatal("missing literal before-side code")
	}
	left, top, _, _ := app.contentBounds()
	y := top + span.row - app.contentViewport.YOffset()
	app = pressMouse(app, left+span.left+1, y)
	app = releaseMouse(app, left+span.left+5, y)
	if got := app.selectedText(); got != "code" {
		t.Fatalf("copied = %q, want literal code body without unselected backquotes", got)
	}
}
