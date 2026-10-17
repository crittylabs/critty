package document

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.md")
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != path || doc.Content != content {
		t.Errorf("Load = %+v", doc)
	}
	// The trailing newline yields an empty last line.
	if doc.LineCount() != 4 {
		t.Errorf("LineCount = %d, want 4", doc.LineCount())
	}
	for line, want := range map[int]string{-1: "", 0: "", 1: "line one", 3: "line three", 4: "", 5: ""} {
		if got := doc.LineAt(line); got != want {
			t.Errorf("LineAt(%d) = %q, want %q", line, got, want)
		}
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Error("loaded a missing file")
	}
}
