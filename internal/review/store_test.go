package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRoundStateJSONRoundTrip(t *testing.T) {
	cj := NewCritJSON()
	cj.ReviewRound = 3
	cj.RoundState = RoundState{
		Verdict: VerdictComment, ReviewCommentDraft: "draft", Finished: true,
		Files: map[string]SnapshotFile{"a.md": {Content: "original"}},
	}
	data, err := json.Marshal(cj)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["critty_state"]; !ok {
		t.Fatal("missing Critty round state")
	}
	var decoded CritJSON
	if err := json.Unmarshal(data, &decoded); err != nil || !reflect.DeepEqual(cj, decoded) {
		t.Fatalf("round trip lost state: %+v, %v", decoded, err)
	}
}

func setupStateDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

func TestSessionRoundTrip(t *testing.T) {
	setupStateDir(t)
	if err := os.WriteFile("plan.md", []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess, err := OpenDocSession("", "plan.md")
	if err != nil {
		t.Fatalf("opening session: %v", err)
	}
	if got := len(sess.FileComments("plan.md")); got != 0 {
		t.Errorf("expected no comments, got %d", got)
	}
	if sess.CJ.ReviewRound != 1 {
		t.Errorf("expected review round 1, got %d", sess.CJ.ReviewRound)
	}

	now := Now()
	multiline := "First line.\nSecond line.\nThird line."
	comment := Comment{
		ID: "c_abc123", StartLine: 1, EndLine: 1, Anchor: "# Test",
		Body: multiline, Author: "Tester", Scope: "line",
		CreatedAt: now, UpdatedAt: now, ReviewRound: 1,
	}
	sess.SetFileComments("plan.md", "", []Comment{comment})
	if err := sess.Save(); err != nil {
		t.Fatalf("saving: %v", err)
	}

	data, err := os.ReadFile(sess.Path())
	if err != nil {
		t.Fatalf("reading review file: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("review file is not valid JSON: %v", err)
	}
	if _, ok := raw["files"]; !ok {
		t.Error("review file missing files map")
	}

	reloaded, err := OpenDocSession("", "plan.md")
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	comments := reloaded.FileComments("plan.md")
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]
	if c.Body != multiline {
		t.Errorf("body mismatch:\ngot:  %q\nwant: %q", c.Body, multiline)
	}
	if c.Author != "Tester" || c.Anchor != "# Test" || c.Scope != "line" {
		t.Errorf("field mismatch: %+v", c)
	}
}

func TestSessionUpdatePreservesConcurrentReplies(t *testing.T) {
	setupStateDir(t)

	sess, err := OpenSession("", "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	sess.SetFileComments("a.go", "", []Comment{
		{ID: "c_first", Body: "first"},
		{ID: "c_second", Body: "second"},
	})
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}

	first, err := OpenSession("", sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenSession("", sess.Key)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	update := func(s *Session, id, body string) {
		<-start
		errs <- s.Update(func(s *Session) error {
			return s.AppendReply(id, body, "AI", "", false, "")
		})
	}
	go update(first, "c_first", "first reply")
	go update(second, "c_second", "second reply")
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("updating session: %v", err)
		}
	}

	reloaded, err := OpenSession("", sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	comments := reloaded.FileComments("a.go")
	if len(comments) != 2 {
		t.Fatalf("comments = %+v, want two", comments)
	}
	if len(comments[0].Replies) != 1 || comments[0].Replies[0].Body != "first reply" {
		t.Errorf("first reply lost: %+v", comments[0].Replies)
	}
	if len(comments[1].Replies) != 1 || comments[1].Replies[0].Body != "second reply" {
		t.Errorf("second reply lost: %+v", comments[1].Replies)
	}
}

func TestSessionKeyPathNormalization(t *testing.T) {
	setupStateDir(t)
	if err := os.WriteFile("plan.md", []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := OpenDocSession("", "plan.md")
	if err != nil {
		t.Fatalf("opening with relative path: %v", err)
	}
	cwd, _ := os.Getwd()
	abs, err := OpenDocSession("", filepath.Join(cwd, "plan.md"))
	if err != nil {
		t.Fatalf("opening with absolute path: %v", err)
	}
	if rel.Key != abs.Key {
		t.Errorf("relative and absolute paths yield different keys: %s vs %s", rel.Key, abs.Key)
	}
	dotted, err := OpenDocSession("", "./plan.md")
	if err != nil {
		t.Fatalf("opening with dotted path: %v", err)
	}
	if rel.Key != dotted.Key {
		t.Errorf("dotted path yields a different key: %s vs %s", rel.Key, dotted.Key)
	}
}

func TestSessionKeyDistinctPerMode(t *testing.T) {
	cwd := "/tmp/project"
	doc := SessionKey(cwd, "", []string{"plan.md"})
	git := SessionKey(cwd, "main", nil)
	other := SessionKey(cwd, "topic", nil)
	if doc == git {
		t.Error("files-mode and git-mode keys should differ")
	}
	if git == other {
		t.Error("keys should differ across branches")
	}
	if len(doc) != 12 || !isHex(doc) {
		t.Errorf("expected 12 hex chars, got %q", doc)
	}
}

func isHex(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdef", r)
	}) < 0
}

func TestSessionClearRemovesFolderAndRegistryEntry(t *testing.T) {
	setupStateDir(t)
	if err := os.WriteFile("plan.md", []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess, err := OpenDocSession("", "plan.md")
	if err != nil {
		t.Fatalf("opening session: %v", err)
	}
	sess.SetFileComments("plan.md", "", []Comment{{ID: "c_abc123", StartLine: 1, EndLine: 1, Body: "x"}})
	if err := sess.Save(); err != nil {
		t.Fatalf("saving: %v", err)
	}

	entries, err := ListSessionEntries()
	if err != nil {
		t.Fatalf("listing sessions: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != sess.Key {
		t.Fatalf("expected one registry entry for %s, got %+v", sess.Key, entries)
	}

	if err := sess.Clear(); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Error("review folder should be removed")
	}
	entries, err = ListSessionEntries()
	if err != nil {
		t.Fatalf("listing sessions after clear: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no registry entries, got %+v", entries)
	}
}
