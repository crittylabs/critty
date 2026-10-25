package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/internal/document"
)

func TestGoOutlineDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []outlineHeading
	}{
		{"declarations", "package p\ntype (\n Server[T any] struct{}\n Alias = int\n)\nfunc New[T any]() {}\nfunc (s *Server[T]) Serve() {}\nfunc (s Server[T]) Close() {}\n", []outlineHeading{{3, 1, "type Server"}, {4, 1, "type Alias"}, {6, 1, "func New"}, {7, 1, "func (*Server[T]) Serve"}, {8, 1, "func (Server[T]) Close"}}},
		{"comments-strings-locals", "package p\n// func Fake() {}\n/* type Fake struct{} */\nvar source = `\nfunc Fake() {}\n`\nfunc Real() {\n type Local int\n f := func() {}\n _ = f\n}\n", []outlineHeading{{7, 1, "func Real"}}},
		{"physical-lines", "package p\n//line generated.go:999\nfunc (s *\nServer) Serve() {}\n", []outlineHeading{{3, 1, "func (*Server) Serve"}}},
		{"fragment", "func Serve() {}\ntype Server struct{}", []outlineHeading{{1, 1, "func Serve"}, {2, 1, "type Server"}}},
		{"recoverable-error", "package p\nfunc Good() {}\nfunc Incomplete() {\n", []outlineHeading{{2, 1, "func Good"}, {3, 1, "func Incomplete"}}},
		{"empty", "package p\nvar x = 1\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseGoOutline(tc.source, 0); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("outline = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestGoOutlineCacheAndPartialSource(t *testing.T) {
	doc := document.FromContent("test.go", []byte("missing\nfunc First() {\nmissing\nmissing\ntype Second struct{}\n"))
	doc.Known = map[int]bool{2: true, 5: true, 6: true}
	tab := FileTab{doc: doc}
	cache := tab.ensureGoOutline()
	want := []outlineHeading{{2, 1, "func First"}, {5, 1, "type Second"}}
	if !reflect.DeepEqual(cache.rows, want) {
		t.Fatalf("partial source = %#v, want %#v", cache.rows, want)
	}
	if tab.ensureGoOutline() != cache {
		t.Fatal("unchanged source was reparsed")
	}
	doc.Content = "package p\nfunc Updated() {}"
	doc.Known = nil
	if got := tab.ensureGoOutline(); got == cache || len(got.rows) != 1 || got.rows[0].label != "func Updated" {
		t.Fatal("modified source did not invalidate the cache")
	}
}

func TestGoOutlineSourceTrackingAndNavigation(t *testing.T) {
	app := outlineTestApp(t, "package p\ntype Server struct{}\n\nfunc (s *Server) Serve() {\n println(1)\n}\n\nfunc Stop() {}\n")
	app.tab().path, app.tab().isMarkdown = "test.go", false
	app.tab().chromaLines = nil
	app.rebuildContent()
	for _, tc := range []struct{ line, active int }{{1, -1}, {2, 0}, {5, 1}, {8, 2}} {
		app.tab().cursorLine = tc.line
		app.rebuildContent()
		if app.outline.active != tc.active {
			t.Fatalf("line %d selected %d, want %d", tc.line, app.outline.active, tc.active)
		}
	}
	left, top, _, _ := app.commentBounds()
	app = clickMouse(app, left+2, top+app.fileTree.viewport.Height()+2)
	if app.tab().cursorLine != 4 || app.tab().cursorOnAnnotation {
		t.Fatal("click did not jump to the Go method")
	}
	app = pressKey(app, 'j')
	if app.tab().cursorLine != 8 {
		t.Fatal("keyboard did not jump to the next Go declaration")
	}
	app, _ = updateApp(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	if app.focused != contentPane {
		t.Fatal("Enter did not return to source")
	}
	app.tab().doc = document.FromContent("test.go", []byte("package p\nfunc Reloaded() {}\n"))
	app.rebuildContent()
	if len(app.outline.rows) != 1 || !strings.Contains(app.renderFilesSidebar(), "Reloaded") {
		t.Fatal("reloaded Go source did not refresh the outline")
	}
	app.tab().path = "test.txt"
	app.rebuildContent()
	if len(app.outline.rows) != 0 {
		t.Fatal("unsupported file retained the Go outline")
	}
}
