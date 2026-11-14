package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/syntax"
)

func TestSyntaxAssociations(t *testing.T) {
	rules, err := syntax.Compile(syntax.Config{
		Aliases: map[string]string{"custom": "python"},
		Files:   []syntax.FileType{{Pattern: "*.note", Language: "markdown"}, {Pattern: "*.demo", Language: "custom"}, {Pattern: "*.md", Language: "text"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := "class Service:\n  def run(self):\n    return 1\n"
	tab := FileTab{path: "sample.demo", syntax: rules, doc: document.FromContent("sample.demo", []byte(source))}
	tab.ensureHighlightCache()
	if !tab.isCode || tab.isMarkdown || ansi.Strip(tab.chromaLines[0]) != "class Service:" || tab.chromaLines[0] == "class Service:" {
		t.Fatalf("custom source was not highlighted: %+v", tab.chromaLines)
	}
	if outline := tab.ensureCodeOutline(); outline == nil || len(outline.rows) != 2 || outline.rows[1].outlineHeading != (outlineHeading{2, 2, "def run"}) {
		t.Fatalf("custom source outline = %+v", outline)
	}

	markdown := "# Heading\n\n```{.CUSTOM title=\"Example\"}\n" + source + "```"
	note := FileTab{path: "sample.note", syntax: rules, doc: document.FromContent("sample.note", []byte(markdown))}
	note.ensureHighlightCache()
	if !note.isMarkdown || note.markdown == nil {
		t.Fatal("custom Markdown format was not rendered")
	}
	assertSyntaxFence(t, note.markdown.after, "CUSTOM", 2, 3)
	if note.doc.Content != markdown {
		t.Fatal("association changed source content")
	}

	plain := FileTab{path: "sample.md", syntax: rules, doc: document.FromContent("sample.md", []byte(markdown))}
	plain.ensureHighlightCache()
	if plain.isMarkdown || plain.isCode || plain.markdown != nil {
		t.Fatal("explicit plaintext rule did not override Markdown detection")
	}

	model := AppModel{syntax: rules, tabs: []FileTab{tab}}
	for _, language := range []string{"custom", "suggestion"} {
		body := "```" + language + "\n" + source + "```"
		parsed := model.parsedComment(body, model.commentSuggestionLexer()).source
		assertSyntaxFence(t, parsed, language, 0, 1)
	}
	// An independent viewer retains built-in detection for the same filename.
	other := FileTab{path: "sample.demo", doc: document.FromContent("sample.demo", []byte(source))}
	other.ensureHighlightCache()
	if other.isCode {
		t.Fatal("configuration leaked into another viewer")
	}
}

func TestSyntaxViewNavigationAndReload(t *testing.T) {
	rules, err := syntax.Compile(syntax.Config{Files: []syntax.FileType{{Pattern: "*.note", Language: "markdown"}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	start := filepath.Join(dir, "start.note")
	next := filepath.Join(dir, "next.note")
	for _, file := range []string{start, next} {
		if err := os.WriteFile(file, []byte("# Heading\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp(start, AppConfig{ReadOnly: true, Syntax: rules})
	app, _ = updateApp(app, docRenderedMsg{})
	app, _ = updateApp(app, tea.WindowSizeMsg{Width: 100, Height: 25})
	if !app.tab().isMarkdown || !app.sidebarVisible() {
		t.Fatal("configured Markdown did not expose its outline")
	}
	followViewTestLink(t, &app, "next.note")
	if !app.tab().isMarkdown || app.tab().syntax != rules {
		t.Fatal("navigation lost syntax rules")
	}
	app.stepViewHistory(false)
	if app.tab().path != start || !app.tab().isMarkdown {
		t.Fatal("Back lost associated Markdown format")
	}
	if err := os.WriteFile(start, []byte("# Changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloadApp(t, &app)
	app.tab().ensureHighlightCache()
	if !app.tab().isMarkdown || app.tab().syntax != rules || !strings.Contains(app.tab().doc.Content, "Changed") {
		t.Fatal("reload lost syntax rules or changed source")
	}
}

func assertSyntaxFence(t *testing.T, source *markdownSource, language string, opening, body int) {
	t.Helper()
	found := false
	for _, token := range source.lines[opening].tokens {
		if token.text == language && token.style == markdownFenceLanguage {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom fence language %q was not accented", language)
	}
	for _, token := range source.lines[body].tokens {
		if token.syntax.InCategory(chroma.Keyword) && strings.Contains(token.text, "class") {
			return
		}
	}
	t.Fatal("custom fence body was not highlighted")
}
