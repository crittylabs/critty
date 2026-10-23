package review

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestRecordSessionProjectPreservesReviewAndMetadata(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sess, err := NewSession("", SessionEntry{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	stale := sess.Meta
	topic := "New topic"
	sess.Meta.Topic, sess.Meta.PID = &topic, 123
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(sess.Path())
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1700000000, 0)
	path := sessionEntryPath(sess.Key)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for _, common := range []string{"/project/.git", "/wrong/.git"} {
		if err := RecordSessionProject(stale, common); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := ReadSessionEntry(sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	if entry.GitCommonDir != "/project/.git" || entry.StartedAt != stale.StartedAt || entry.PID != 123 || entry.Topic == nil || *entry.Topic != topic {
		t.Fatalf("metadata changed during enrichment: %+v", entry)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("registry timestamp changed: %v, %v", info, err)
	}
	after, err := os.ReadFile(sess.Path())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("review changed during enrichment: %v", err)
	}
	// A process opened before enrichment must not erase the discovered identity.
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	entry, err = ReadSessionEntry(sess.Key)
	if err != nil || entry.GitCommonDir != "/project/.git" {
		t.Fatalf("later save erased project identity: %+v, %v", entry, err)
	}
}
