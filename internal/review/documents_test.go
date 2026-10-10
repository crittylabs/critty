package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentAttachmentSnapshots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notice.md")
	if err := os.WriteFile(path, []byte("# Notice\nOriginal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := ReadAttachedDocuments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenSession(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocuments(docs); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(map[string]SnapshotFile{docs[0].Path(): {Content: docs[0].Content}}, ""); err != nil {
		t.Fatal(err)
	}
	s.SetFileComments(docs[0].Path(), "", []Comment{{ID: "c_test", StartLine: 2, Body: "Change this", Anchor: "Original"}})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Notice\nInserted\nOriginal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved, err := s.LoadDocuments()
	if err != nil || saved.Documents[0].Content != docs[0].Content {
		t.Fatalf("snapshot changed with source: %+v, %v", saved, err)
	}
	revised, err := ReadAttachedDocuments([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if revised[0].ID != docs[0].ID || revised[0].Revision == docs[0].Revision {
		t.Fatal("revision changed document identity or retained content hash")
	}
	if err := s.ReplaceDocuments(revised); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(map[string]SnapshotFile{docs[0].Path(): {Content: revised[0].Content}}, ""); err != nil {
		t.Fatal(err)
	}
	if got := s.FileComments(docs[0].Path()); len(got) != 1 || got[0].StartLine != 3 {
		t.Fatalf("comment not carried onto revision: %+v", got)
	}
	if err := s.ReplaceDocuments(nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	saved, err = s.LoadDocuments()
	if err != nil || len(saved.Documents) != 1 || !saved.Documents[0].Removed || saved.Documents[0].Content != revised[0].Content {
		t.Fatalf("removed snapshot lost: %+v, %v", saved, err)
	}
}

func TestDocumentAttachmentNamesAndValidation(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"server/docs/README.md", "client/docs/README.md", "NOTICE.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	docs, err := ReadAttachedDocuments(paths)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"server/docs/README.md", "client/docs/README.md", "NOTICE.txt"} {
		if docs[i].Name != want {
			t.Fatalf("document %d name: %q, want %q", i, docs[i].Name, want)
		}
	}
	alias := filepath.Join(dir, "alias.md")
	if err := os.Symlink(paths[0], alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAttachedDocuments([]string{paths[0], alias}); err == nil {
		t.Fatal("duplicate via symlink accepted")
	}
	for _, data := range [][]byte{{0xff}, {'a', 0, 'b'}} {
		if err := os.WriteFile(alias, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadAttachedDocuments([]string{alias}); err == nil {
			t.Fatal("binary accepted")
		}
	}
	if _, err := ReadAttachedDocuments([]string{dir}); err == nil {
		t.Fatal("directory accepted")
	}
}
