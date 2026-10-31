package searchhistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushDeduplicatesAndTrims(t *testing.T) {
	entries := []Entry{{Query: "a"}, {Query: "b", Mode: "word"}, {Query: "c"}}
	got := Push(entries, Entry{Query: "b", Mode: "word"})
	if len(got) != 3 || got[2].Query != "b" || got[0].Query != "a" || got[1].Query != "c" {
		t.Fatalf("Push moved the duplicate wrongly: %+v", got)
	}
	got = Push(got, Entry{Query: "b"})
	if len(got) != 4 {
		t.Fatalf("an entry with different options should be distinct: %+v", got)
	}
	for i := range Limit {
		got = Push(got, Entry{Query: strings.Repeat("x", i+1)})
	}
	if len(got) != Limit || got[0].Query != "x" {
		t.Fatalf("len = %d, first = %q", len(got), got[0].Query)
	}
}

func TestStoreAddAndLoadByProject(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := Open("git:/repo/.git")
	if entries, err := store.Load(); err != nil || entries != nil {
		t.Fatalf("Load before any save = %v, %v", entries, err)
	}
	if _, err := store.Add(Entry{Query: "alpha", Mode: "literal", CaseFold: true}); err != nil {
		t.Fatal(err)
	}
	other := Open("directory:/elsewhere")
	if _, err := other.Add(Entry{Query: "beta"}); err != nil {
		t.Fatal(err)
	}
	entries, err := Open("git:/repo/.git").Load()
	if err != nil || len(entries) != 1 || entries[0] != (Entry{Query: "alpha", Mode: "literal", CaseFold: true}) {
		t.Fatalf("Load = %+v, %v", entries, err)
	}
	info, err := os.Stat(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	if filepath.Base(store.Path) != "isearch-history.json" {
		t.Fatalf("path = %s", store.Path)
	}
}

func TestStoreKeepsCorruptFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store := Open("git:/repo/.git")
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(Entry{Query: "alpha"}); err == nil {
		t.Fatal("Add overwrote a corrupt history file")
	}
	data, _ := os.ReadFile(store.Path)
	if string(data) != "{broken" {
		t.Fatalf("file changed to %q", data)
	}
}
