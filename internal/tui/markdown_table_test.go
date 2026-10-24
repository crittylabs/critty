package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestWrapTableCells(t *testing.T) {
	const uri = "https://example.org/path"
	cells := []string{linkedText(uri, "long linked label"), "日本語 é and text"}
	widths := []int{6, 8}
	columns := wrapTableCells(cells, widths)
	if tableRowHeight(columns) < 2 {
		t.Fatal("cells did not wrap")
	}
	for row := range tableRowHeight(columns) {
		rendered := formatTableRow(tableRowCells(columns, row), widths, false)
		if got := lipgloss.Width(rendered); got != 21 {
			t.Fatalf("row %d width = %d, want 21: %q", row, got, rendered)
		}
		if strings.Count(ansi.Strip(rendered), "│") != 3 {
			t.Fatalf("row %d lost table borders: %q", row, rendered)
		}
	}
	for _, fragment := range columns[0] {
		if !strings.Contains(fragment.text, ansi.SetHyperlink(uri)) {
			t.Fatalf("wrapped link lost destination: %q", fragment.text)
		}
	}
}

func TestWrappedMarkdownTableSelection(t *testing.T) {
	const label = "first one two three four five six seven eight nine last"
	const link = "[" + label + "](https://example.org/path)"
	const note = "**alpha bravo charlie delta echo foxtrot golf hotel omega**"
	const content = "| Link | Notes |\n| --- | --- |\n| " + link + " | " + note + " |"
	for _, selection := range []string{"link", "note", "whole"} {
		t.Run(selection, func(t *testing.T) {
			app := setupAppWithDoc(t, content)
			app.width, app.height, app.viewMode = 200, 40, true
			app.recalculateLayout()
			app.rebuildContent()
			spans := app.contentLayout.textSpans
			first, last := spans[0], spans[len(spans)-1]
			start, end, want := 0, len(last.text), content
			if selection != "whole" {
				a, b := "first", "last"
				want = link
				if selection == "note" {
					a, b, want = "alpha", "omega", note
				}
				for _, span := range spans {
					if at := strings.Index(span.text, a); at >= 0 {
						first, start = span, at
					}
					if at := strings.Index(span.text, b); at >= 0 {
						last, end = span, at+len(b)
					}
				}
				if first.row == last.row {
					t.Fatal("test selection did not cross wrapped cell rows")
				}
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

func TestMarkdownTableKeepsColumnsWhenResized(t *testing.T) {
	content := "| PR | Notes |\n| --- | --- |\n| [日本語 é long label](https://example.org) | " + strings.Repeat("words ", 20) + " |"
	for _, width := range []int{80, 110} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			app := setupAppWithDoc(t, content)
			app.width, app.height = width, 40
			app.recalculateLayout()
			app.rebuildContent()
			rows := strings.Split(app.contentViewport.GetContent(), "\n")
			count, rowWidth := 0, 0
			for i, target := range app.contentLayout.rows {
				if target.line != 3 {
					continue
				}
				plain := ansi.Strip(rows[i])
				if !strings.Contains(plain, "│") {
					t.Fatalf("table borders missing: %q", plain)
				}
				w := ansi.StringWidth(rows[i])
				if count > 0 && w != rowWidth || w > app.contentViewport.Width() {
					t.Fatalf("row width = %d, previous = %d, viewport = %d", w, rowWidth, app.contentViewport.Width())
				}
				count++
				rowWidth = w
			}
			if count < 2 {
				t.Fatal("table did not wrap")
			}
		})
	}
}
