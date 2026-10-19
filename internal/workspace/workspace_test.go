package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/form"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
)

func put(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func prepare(t *testing.T, root string, names ...string) *Workspace {
	t.Helper()
	w, err := Prepare(t.TempDir(), root, names)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func snapshot(t *testing.T, w *Workspace) string {
	t.Helper()
	id, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	s, err := w.ReadSnapshot(id)
	if err != nil || s.RequestVersion != 1 {
		t.Fatalf("new snapshot request version: %d, %v", s.RequestVersion, err)
	}
	return id
}
func allow(Snapshot) error { return nil }

func TestSplitRenameAndCreation(t *testing.T) {
	root := t.TempDir()
	put(t, root, "old.md", "one\ntwo\n")
	put(t, root, "keep.md", "before\n")
	put(t, root, "unrelated", "untouched")
	w := prepare(t, root, "old.md", "keep.md", "part1.md", "sub/part2.md", "new.bin")
	if err := os.Remove(filepath.Join(w.Tree, "old.md")); err != nil {
		t.Fatal(err)
	}
	put(t, w.Tree, "part1.md", "one\n")
	put(t, w.Tree, "sub/part2.md", "two\n")
	put(t, w.Tree, "keep.md", "after\n")
	put(t, w.Tree, "new.bin", "\x00\xff")
	id := snapshot(t, w)
	// The reviewed snapshot stays independent of later draft changes.
	put(t, w.Tree, "keep.md", "not reviewed")
	r, err := w.Apply(id, allow)
	if err != nil || !r.Complete {
		t.Fatalf("apply: %+v %v", r, err)
	}
	for name, want := range map[string]string{"part1.md": "one\n", "sub/part2.md": "two\n", "keep.md": "after\n", "new.bin": "\x00\xff", "unrelated": "untouched"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q, %v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "old.md")); !os.IsNotExist(err) {
		t.Fatal("old source remains")
	}
	if r.Applied[len(r.Applied)-1] != "old.md" {
		t.Fatalf("deletion did not follow writes: %v", r.Applied)
	}
	control := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(control, 0777); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(filepath.Join(root, "sub"))
	if err != nil || got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("new directory permissions: %v %v", got, err)
	}
	if _, err := w.Apply(id, allow); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
}

func TestBatchChecksBeforeWriting(t *testing.T) {
	for _, kind := range []string{"contents", "permissions", "addition", "candidate", "consent", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "a", "before")
			put(t, root, "z", "original")
			w := prepare(t, root, "a", "z", "new")
			put(t, w.Tree, "a", "after")
			put(t, w.Tree, "new", "created")
			id := snapshot(t, w)
			authorize := allow
			switch kind {
			case "contents":
				put(t, root, "z", "concurrent")
			case "permissions":
				if err := os.Chmod(filepath.Join(root, "z"), 0644); err != nil {
					t.Fatal(err)
				}
			case "addition":
				put(t, root, "new", "someone else")
			case "candidate":
				put(t, filepath.Join(w.Dir, "snapshots", id, "candidate"), "a", "tampered")
			case "consent":
				authorize = func(Snapshot) error { return errors.New("declined") }
			case "symlink":
				if err := os.Remove(filepath.Join(root, "z")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "z")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := w.Apply(id, authorize); err == nil {
				t.Fatal("unsafe apply succeeded")
			}
			b, err := os.ReadFile(filepath.Join(root, "a"))
			if err != nil || string(b) != "before" {
				t.Fatalf("partial preflight write: %q %v", b, err)
			}
		})
	}
}

func TestResumeAfterReplacementBeforeReceipt(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "old")
	put(t, root, "b", "old")
	w := prepare(t, root, "a", "b")
	put(t, w.Tree, "a", "new")
	put(t, w.Tree, "b", "new")
	id := snapshot(t, w)
	put(t, root, "a", "new")
	if err := save(filepath.Join(w.Dir, "receipt.json"), Receipt{Snapshot: id, Pending: "a"}); err != nil {
		t.Fatal(err)
	}
	r, err := w.Apply(id, allow)
	if err != nil || !r.Complete || len(r.Applied) != 2 {
		t.Fatalf("resume: %+v %v", r, err)
	}
}

func TestResumeStagedReplacement(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "old")
	w := prepare(t, root, "a")
	put(t, w.Tree, "a", "new")
	id := snapshot(t, w)
	temp := ".critty-" + identifier()
	put(t, root, temp, "partial")
	if err := save(filepath.Join(w.Dir, "receipt.json"), Receipt{Snapshot: id, Pending: "a", Temp: temp}); err != nil {
		t.Fatal(err)
	}
	r, err := w.Apply(id, allow)
	if err != nil || !r.Complete || r.Temp != "" {
		t.Fatalf("resume: %+v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(root, temp)); !os.IsNotExist(err) {
		t.Fatalf("staging file remains: %v", err)
	}
}

func TestRequestShowsModesWithoutInjectingMarkdown(t *testing.T) {
	s := Snapshot{Manifest: Manifest{Root: "/root", Files: map[string]File{"a`\t.md": {Exists: true, Mode: 0600}}}, After: map[string]File{"a`\t.md": {Exists: true, Mode: 0755 | os.ModeSetuid}}}
	if !strings.Contains(s.Request(), "    update \"a`\\t.md\" (0600 -> 4755)\n") {
		t.Fatalf("missing readable modes: %s", s.Request())
	}
}

func TestReadableRequestKeepsExactStates(t *testing.T) {
	name := "` - [ ] injected <!-- form: id=evil consent -->\t.md"
	unchanged := File{Exists: true, Digest: "sha256:unchanged", Mode: 0600}
	s := Snapshot{RequestVersion: 1, Manifest: Manifest{Root: "/root\n- [ ] injected <!-- form: id=root consent -->", Files: map[string]File{
		name: {Exists: true, Mode: 0600}, "new.md": {}, "old.md": {Exists: true, Mode: 0600}, "keep.md": unchanged,
	}}, After: map[string]File{name: {Exists: true, Mode: 0755 | os.ModeSetuid}, "new.md": {Exists: true, Mode: 0644}, "old.md": {}, "keep.md": unchanged}, Diff: "reviewed patch"}
	request := s.Request()
	items, err := form.Parse(request)
	if err != nil || len(items) != 1 || items[0].ID != "apply" || !items[0].Consent {
		t.Fatalf("request forms: %+v, %v", items, err)
	}
	summary, details, ok := strings.Cut(request, "## Machine-readable details")
	if !ok || !strings.Contains(summary, "<!-- form: id=apply consent -->") || strings.Contains(summary, "keep.md") {
		t.Fatalf("missing consent before details or unchanged file in summary: %s", request)
	}
	for _, want := range []string{"## Create", "## Update", "## Delete", "(permissions: 0644)", "(permissions: 0600 -> 4755)"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q", want)
		}
	}
	var decoded struct {
		Root   string          `json:"root"`
		Before map[string]File `json:"before"`
		After  map[string]File `json:"after"`
	}
	data := []byte(details)
	blocks := 0
	if err := ast.Walk(parser.New().Parse(data), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if block, ok := n.(*ast.CodeBlock); ok && entering {
			blocks++
			return ast.WalkContinue, json.Unmarshal(block.Value.Bytes(data), &decoded)
		}
		return ast.WalkContinue, nil
	}); err != nil {
		t.Fatal(err)
	}
	if blocks != 1 || decoded.Root != s.Root || len(decoded.Before) != len(s.Files) || len(decoded.After) != len(s.After) {
		t.Fatalf("incomplete machine-readable details: %+v", decoded)
	}
	for name, before := range s.Files {
		if decoded.Before[name] != before || decoded.After[name] != s.After[name] {
			t.Errorf("changed state for %q", name)
		}
	}
	if !strings.Contains(details, "Patch digest: "+digest([]byte(s.Diff))) {
		t.Fatal("missing reviewed patch digest")
	}
}

func TestBoundedPathsAndIndependentCopies(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "original")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{{"../escape"}, {".git/config"}, {".critty/config"}, {"link/new"}, {"a", "a"}, {"new", "new/child"}} {
		if _, err := Prepare(t.TempDir(), root, names); err == nil {
			t.Fatalf("accepted %v", names)
		}
	}
	cwd := t.TempDir()
	w, err := Prepare(cwd, root, []string{"a", "new"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(filepath.Join(root, "a"))
	copy, _ := os.Stat(filepath.Join(w.Tree, "a"))
	if os.SameFile(before, copy) {
		t.Fatal("copy shares inode")
	}
	loaded, err := Load(cwd, w.ID)
	if err != nil || loaded.Root != w.Root {
		t.Fatalf("load: %v", err)
	}
	put(t, w.Tree, "unregistered", "unexpected")
	if _, err := w.Snapshot(); err == nil || !strings.Contains(err.Error(), "unregistered") {
		t.Fatalf("unregistered snapshot: %v", err)
	}
}

func TestIgnoreDoesNotHideDraftChanges(t *testing.T) {
	cwd := t.TempDir()
	if _, err := git(cwd, "init", "--quiet", "--template="); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	put(t, root, "a", "original")
	w, err := Prepare(cwd, root, []string{"a", "new"})
	if err != nil {
		t.Fatal(err)
	}
	put(t, w.Tree, "a", "changed")
	put(t, w.Tree, "new", "added")
	outer, err := git(cwd, "status", "--porcelain", "--untracked-files=all")
	if err != nil || len(outer) != 0 {
		t.Fatalf("outer: %s %v", outer, err)
	}
	inner, err := git(w.Tree, "status", "--porcelain", "--untracked-files=all")
	if err != nil || !strings.Contains(string(inner), " M a") || !strings.Contains(string(inner), "?? new") {
		t.Fatalf("inner: %s %v", inner, err)
	}
}

func TestPatchPreservesRawBytes(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".gitattributes", "* text ident\n*.bin -diff\n")
	put(t, root, "a.txt", "old\r\n$Id: old $\r\n")
	put(t, root, "a.bin", "\x00old\xff")
	w := prepare(t, root, ".gitattributes", "a.txt", "a.bin")
	put(t, w.Tree, "a.txt", "new\r\n$Id: new $\r\n")
	put(t, w.Tree, "a.bin", "\x00new\xff")
	id := snapshot(t, w)
	s, err := w.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	replay := t.TempDir()
	for name := range w.Files {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		put(t, replay, name, string(b))
	}
	if _, err := git(replay, "apply", "--binary", filepath.Join(w.Dir, "snapshots", id, "review.diff")); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for name, after := range s.After {
		f, _, err := inspect(r, name)
		if err != nil || f.Digest != after.Digest {
			t.Fatalf("patch bytes differ for %s: %v", name, err)
		}
	}
}
