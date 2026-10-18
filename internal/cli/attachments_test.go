package cli

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/review"
)

func TestAttachmentApprovalCleanup(t *testing.T) {
	sess := newPayloadSession(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sess.Meta = review.SessionEntry{Key: sess.Key, CWD: cwd, Mode: "files"}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	path, err := sess.SaveAttachment(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	sess.SetFileComments("a.go", "", []review.Comment{{ID: "c_1", Body: "fixed", Resolved: true, Replies: []review.Reply{{ID: "r_1", Body: "see ![screenshot](" + path + ")"}}}})
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{CleanupOnApprove: true}
	for _, approved := range []bool{false, true} {
		payload := buildFinishPayload(cfg, sess, &reviewMode{}, approved)
		if !strings.Contains(payload.Prompt, sess.Dir) || !strings.Contains(payload.Prompt, path) {
			t.Fatal("missing image context in finish output")
		}
		if strings.Contains(payload.Prompt, "critty clear --session "+sess.Key) != approved {
			t.Fatal("incorrect cleanup instruction")
		}
	}
	cleanupOnApprove(cfg, sess)
	if _, err := os.Stat(filepath.Join(sess.Dir, path)); err != nil {
		t.Fatal("approval deleted unread image:", err)
	}
	if err := clearSavedReview(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Fatalf("completed review remains: %v", err)
	}
}
