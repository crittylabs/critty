package review

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/crittylabs/critty/internal/form"
)

func TestMemoryReviewLifecycle(t *testing.T) {
	root := t.TempDir()
	s := NewMemorySession()
	s.Dir = filepath.Join(root, "must-not-exist")
	docs := []AttachedDocument{{ID: "demo", Source: "/demo.md", Name: "demo.md", Content: "- [ ] Confirm <!-- form: id=confirm required -->"}}
	if err := s.ReplaceDocuments(docs); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAnswerMode(true); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginRound(map[string]SnapshotFile{"target.md": {Content: "target"}}, ""); err != nil {
		t.Fatal(err)
	}
	s.AppendFileComment("target.md", "demo comment", "Tester", "")
	state, err := s.BeginForms(ReviewInput{Scope: "document"}, docs)
	if err != nil {
		t.Fatal(err)
	}
	state.Documents[0].Answers["confirm"] = form.Answer{Value: true, Human: true}
	if err := s.SaveForms(state); err != nil {
		t.Fatal(err)
	}
	s.SetSubmission(VerdictComment, "demo feedback", "Tester")
	s.CJ.RoundState.Finished = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFormResult(state.Result(false, 1)); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadForms()
	if err != nil || !loaded.Documents[0].Answers["confirm"].Human {
		t.Fatalf("lost answer: %+v, %v", loaded, err)
	}
	attachments, err := s.LoadDocuments()
	if err != nil || !attachments.Answers || len(attachments.Documents) != 1 {
		t.Fatalf("lost documents: %+v, %v", attachments, err)
	}
	if len(s.FileComments("target.md")) != 1 || s.SubmissionComment().Body != "demo feedback" {
		t.Fatal("lost feedback")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("memory review wrote files: %v, %v", entries, err)
	}
}

func TestMemoryReviewImagesAreTemporary(t *testing.T) {
	s := NewMemorySession()
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path, err := s.SaveAttachment(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(s.Dir, path))
	if err != nil || !bytes.Equal(saved, data.Bytes()) {
		t.Fatalf("image unavailable: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("demo image caused review persistence: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Fatalf("temporary images survived close: %v", err)
	}
}
