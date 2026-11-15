package tui

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCommentCodeHighlighting(t *testing.T) {
	app := NewApp("main.go", AppConfig{})
	app.images.hidden = true
	for _, language := range []string{"go", "suggestion", "console", "shell-session", "md", "markdown"} {
		t.Run(language, func(t *testing.T) {
			code := "var count = 42"
			if language == "console" || language == "shell-session" {
				code = "$ echo \"hello\"\nhello"
			} else if language == "md" || language == "markdown" {
				code = "# Heading\n**bold** and `code`"
			}
			body := "```" + language + "\n" + code + "\n```"
			parsed := app.commentRenderedDisplay(body, 80, false)
			if !slices.ContainsFunc(parsed.lines[1].tokens, func(token markdownToken) bool { return token.syntax != 0 }) {
				t.Fatal("code in comments has no syntax tokens with images hidden")
			}
			view := app.renderCommentContent("Reviewer", body, "", 80)
			plainRows := strings.Split(ansi.Strip(view), "\n")
			for i := range plainRows {
				plainRows[i] = strings.TrimRight(plainRows[i], " ")
			}
			if !strings.Contains(strings.Join(plainRows, "\n"), code) {
				t.Fatalf("code changed during comment rendering: %q", view)
			}
			want := parsed.lines[1].render(nil, lipgloss.NewStyle(), lipgloss.NewStyle(), nil)
			_, colors := renderedCells(want)
			_, rendered := renderedCells(view)
			start := strings.Index(ansi.Strip(view), strings.Split(code, "\n")[0])
			start = len([]rune(strings.ReplaceAll(ansi.Strip(view)[:start], "\n", "")))
			for i, color := range colors {
				if !reflect.DeepEqual(color.Fg, rendered[start+i].Fg) {
					t.Fatalf("code foreground lost at character %d: %v -> %v (%q)", i, color.Fg, rendered[start+i].Fg, view)
				}
			}
		})
	}
}

func TestCommentMarkdownPreservesAuthorColors(t *testing.T) {
	previousDark := terminalIsDark
	t.Cleanup(func() { initAdaptiveStyles(previousDark) })
	const body = "plain **bold** *italic* after\n`code` [link](https://example.org) <kbd>Enter</kbd>"
	for _, dark := range []bool{false, true} {
		initAdaptiveStyles(dark)
		app := NewApp("notes.md", AppConfig{Author: "Self"})
		app.images.hidden = true
		for _, author := range []string{"Alice", "Bob", "Alice", "Self"} {
			view := app.renderCommentContent(author, body, "", 80)
			runes, styles := renderedCells(view)
			plain := string(runes)
			for _, label := range []string{"plain", "bold", "italic", "after", "code", "link", "Enter"} {
				start := strings.Index(plain, label)
				if start < 0 {
					t.Fatalf("label missing: %s", label)
				}
				start = len([]rune(plain[:start]))
				want := app.commentAuthorStyle(author).GetForeground()
				switch label {
				case "code", "Enter":
					want = mdCodeStyle.GetForeground()
				case "link":
					want = mdLinkStyle.GetForeground()
				}
				for i := start; i < start+len(label); i++ {
					if !sameColor(styles[i].Fg, want) {
						t.Fatalf("dark=%t author=%s label=%s: foreground=%v, want=%v", dark, author, label, styles[i].Fg, want)
					}
				}
			}
		}
	}
}

func TestSuggestionLanguageCache(t *testing.T) {
	app := NewApp("main.go", AppConfig{})
	const body = "```suggestion\ndef total(values):\n    return sum(values)\n```"
	goSource := app.commentRenderedDisplay(body, 80, false)
	app.tab().path = "main.py"
	pythonSource := app.commentRenderedDisplay(body, 80, false)
	if goSource == pythonSource {
		t.Fatal("suggestion syntax cache reused another target language")
	}
	found := false
	for _, token := range pythonSource.lines[1].tokens {
		if token.text == "def" && token.syntax.InCategory(chroma.Keyword) {
			found = true
		}
	}
	if !found {
		t.Fatal("suggestion did not inherit Python syntax from its target")
	}
}

func TestEditorCodeHighlightingPreservesLayout(t *testing.T) {
	previous := terminalIsDark
	t.Cleanup(func() { terminalIsDark = previous })
	for _, dark := range []bool{false, true} {
		terminalIsDark = dark
		for _, width := range []int{12, 60} {
			for _, selected := range []bool{false, true} {
				t.Run(fmt.Sprintf("dark=%t/width=%d/selected=%t", dark, width, selected), func(t *testing.T) {
					app := NewApp("main.go", AppConfig{})
					app.modal = commentModal
					app.modalTextarea.SetWidth(width)
					app.modalTextarea.SetHeight(6)
					app.modalTextarea.Focus()
					app.modalTextarea.SetValue("```suggestion\nvar count = 42\nvar name = \"界 é value with spaces\"\n```")
					app.modalTextarea.MoveToBegin()
					if selected {
						app.modalTextarea.SelectAll()
					}
					before := app.modalTextarea.View()
					cursor := app.modalTextarea.Cursor()
					value := app.modalTextarea.Value()
					after, _ := app.editorView()
					if ansi.Strip(before) != ansi.Strip(after) || !reflect.DeepEqual(cursor, app.modalTextarea.Cursor()) || value != app.modalTextarea.Value() {
						t.Fatalf("highlighting changed editor layout, cursor or source:\n%q\n%q", before, after)
					}
					_, oldStyles := renderedCells(before)
					_, newStyles := renderedCells(after)
					colored := false
					for i := range oldStyles {
						if !reflect.DeepEqual(oldStyles[i].Fg, newStyles[i].Fg) {
							colored = true
							if selected {
								t.Fatalf("selection foreground changed at %d", i)
							}
						}
						if !reflect.DeepEqual(oldStyles[i].Bg, newStyles[i].Bg) {
							t.Fatalf("editor background changed at %d", i)
						}
					}
					if !selected && !colored {
						t.Fatal("code was not highlighted while editing")
					}
				})
			}
		}
	}
}

func TestEditorCodeHighlightingAfterScrollAndEdit(t *testing.T) {
	app := NewApp("main.py", AppConfig{})
	app.modal = replyModal
	app.modalTextarea.SetWidth(20)
	app.modalTextarea.SetHeight(3)
	app.modalTextarea.Focus()
	app.modalTextarea.SetValue("Intro\n\n```suggestion\ndef total(values):\n    return sum(values) + 42\n```")
	app.modalTextarea.MoveToEnd()
	for _, suffix := range []string{"", "\n# End"} {
		if suffix != "" {
			app.modalTextarea.InsertString(suffix)
		}
		app.modalTextarea, _ = app.modalTextarea.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
		before := app.modalTextarea.View()
		cursor := app.modalTextarea.Cursor()
		after, _ := app.editorView()
		if ansi.Strip(after) != ansi.Strip(before) || !reflect.DeepEqual(cursor, app.modalTextarea.Cursor()) {
			t.Fatal("scrolled editor lost text or moved its cursor")
		}
		if app.modalTextarea.ScrollYOffset() == 0 {
			t.Fatal("test did not scroll the textarea")
		}
	}
}
