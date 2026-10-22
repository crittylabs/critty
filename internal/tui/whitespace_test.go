package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/review"
)

const whitespacePatch = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-foo()\n-old()\n+  foo()\n+new()\n" +
	"diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -10 +10 @@\n-bar()\r\n+\tbar()\n"

func TestWhitespaceToggleAcrossFiles(t *testing.T) {
	m := patchApp(t, whitespacePatch)
	if !strings.Contains(ansi.Strip(m.renderHeader()), "Ignore: none") {
		t.Fatal("whitespace control should always be present")
	}
	before := m.tabs[0].doc.Content
	m.tab().cursorLine, m.tab().cursorSide = 1, "old"
	m.tab().selecting, m.tab().selectSide = true, "old"
	m = pressKey(m, 'w')
	if !m.ignoreWhitespace || !strings.Contains(ansi.Strip(m.renderHeader()), "Ignore: WS") {
		t.Fatal("missing whitespace mode")
	}
	if m.tabs[0].changedLines[1] || !m.tabs[0].changedLines[2] || len(m.tabs[1].changeChunks) != 0 {
		t.Fatal("toggle did not filter every tab")
	}
	if m.tab().cursorSide != "" || m.tab().selecting {
		t.Fatal("cursor or selection left on hidden old line")
	}
	if m.tabs[0].doc.Content != before {
		t.Fatal("source content changed")
	}
	m = pressKey(m, '2')
	if m.activeTab != 1 || len(m.tab().changedLines) != 0 {
		t.Fatal("tab switch lost mode")
	}
	m = pressKey(m, 'w')
	for _, tab := range m.tabs {
		if !reflect.DeepEqual(tab.changedLines, tab.diff.ChangedLines) || !reflect.DeepEqual(tab.deletedAfter, tab.diff.DeletedAfter) {
			t.Fatal("original diff not restored")
		}
	}
	if m.ignoreWhitespace || !strings.Contains(ansi.Strip(m.renderHeader()), "Ignore: none") {
		t.Fatal("mode not cleared")
	}
}

func TestWhitespaceControlKeepsHeaderPositions(t *testing.T) {
	m := patchApp(t, whitespacePatch)
	_, before := m.renderReviewScreen()
	if !before.hasWhitespaceButton {
		t.Fatal("missing whitespace button")
	}
	m = clickMouse(m, before.whitespaceButton.left, before.whitespaceButton.top)
	_, after := m.renderReviewScreen()
	if !m.ignoreWhitespace || !strings.Contains(ansi.Strip(m.renderHeader()), "Ignore: WS") {
		t.Fatal("click did not toggle whitespace")
	}
	if before.whitespaceButton != after.whitespaceButton || before.splitButton != after.splitButton || !reflect.DeepEqual(before.isearchRegions, after.isearchRegions) {
		t.Fatal("toggling whitespace moved header controls")
	}
	m = pressKey(m, 'w')
	if m.ignoreWhitespace {
		t.Fatal("w did not restore whitespace")
	}
}

func TestWhitespaceToggleKeepsOldComments(t *testing.T) {
	m := patchApp(t, whitespacePatch)
	m.tabs[1].state.Comments = []review.Comment{{ID: "old", Side: "old", StartLine: 10, EndLine: 10, Body: "keep this"}}
	m = pressKey(m, 'w')
	if len(m.tabs[1].deletedAfter[9]) != 1 || len(m.tabs[1].changeChunks) != 0 {
		t.Fatal("comment context lost or counted as a change")
	}
	m = pressKey(m, ']')
	if m.activeTab != 1 || m.tab().cursorSide != "old" || m.tab().cursorLine != 10 || !m.tab().cursorOnAnnotation {
		t.Fatal("old comment unreachable")
	}
	if !strings.Contains(m.contentViewport.GetContent(), "keep this") {
		t.Fatal("comment not rendered")
	}
}

func TestWhitespaceKeyInInput(t *testing.T) {
	for _, mode := range []string{"comment", "search", "help"} {
		t.Run(mode, func(t *testing.T) {
			m := patchApp(t, whitespacePatch)
			switch mode {
			case "comment":
				m.openLineComment()
			case "search":
				m, _ = updateApp(m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt})
			case "help":
				m = pressAltKey(m, '?')
			}
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
			switch v := updated.(type) {
			case AppModel:
				m = v
			case *AppModel:
				m = *v
			}
			if m.ignoreWhitespace {
				t.Fatal("input toggled whitespace mode")
			}
			if mode == "comment" && m.modalTextarea.Value() != "w" {
				t.Fatal("w not entered")
			}
			if mode == "search" && m.fileSelect.input.Value() != "w" {
				t.Fatal("w not searched")
			}
		})
	}
}
