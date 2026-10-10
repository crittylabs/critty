package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/review"
)

func TestPrepareDocumentAttachments(t *testing.T) {
	oldPaths, oldClear, oldFocus := attachmentPaths, clearAttachments, reviewFocus
	t.Cleanup(func() { attachmentPaths, clearAttachments, reviewFocus = oldPaths, oldClear, oldFocus })
	path := filepath.Join(t.TempDir(), "notice.md")
	if err := os.WriteFile(path, []byte("# Notice\nReview me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	attachmentPaths, clearAttachments = []string{path}, false
	mode := &reviewMode{docPath: "other.md"}
	if err := prepareDocuments(mode); err != nil {
		t.Fatal(err)
	}
	reviewFocus = path + ":2"
	if err := resolveFocus(mode); err != nil || mode.focusPath != mode.documents[0].Path() || mode.focusLine != 2 {
		t.Fatalf("attachment focus: %+v, %v", mode, err)
	}
	reviewFocus = path + ":20"
	if err := resolveFocus(mode); err == nil {
		t.Fatal("focus outside snapshot accepted")
	}
	clearAttachments = true
	if err := prepareDocuments(&reviewMode{sessionKey: "saved"}); err == nil {
		t.Fatal("conflicting flags accepted")
	}
	attachmentPaths = nil
	if err := prepareDocuments(&reviewMode{}); err == nil {
		t.Fatal("clear without saved session accepted")
	}
	mode = &reviewMode{sessionKey: "saved"}
	if err := prepareDocuments(mode); err != nil || !mode.replaceDocuments || len(mode.documents) != 0 {
		t.Fatalf("clear did not replace list: %+v, %v", mode, err)
	}
}

func TestDocumentAttachmentsFinishPrompt(t *testing.T) {
	dir := t.TempDir()
	sess, err := review.OpenSession(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	d := review.AttachedDocument{ID: "a_notice", Name: "notice.md", Source: "/original/notice.md", Content: "Exact reviewed text"}
	if err := sess.ReplaceDocuments([]review.AttachedDocument{d}); err != nil {
		t.Fatal(err)
	}
	payload := buildFinishPayload(&config.Config{}, sess, &reviewMode{docPath: "plan.md"}, true)
	for _, want := range []string{d.Path(), d.Source, d.Content, "--clear-attachments"} {
		if !strings.Contains(payload.Prompt, want) {
			t.Fatalf("finish prompt missing %q: %s", want, payload.Prompt)
		}
	}
	if !payload.Approved {
		t.Fatal("attachment changed approval")
	}
}
