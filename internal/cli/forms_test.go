package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

func TestFormFinishContractAndCleanup(t *testing.T) {
	sess, err := review.OpenSession(t.TempDir(), "forms")
	if err != nil {
		t.Fatal(err)
	}
	state, err := sess.BeginForms(review.ReviewInput{}, []review.AttachedDocument{{ID: "a", Source: "/form.md", Name: "form.md", Content: "- [ ] Publish <!-- form: id=publish consent -->"}})
	if err != nil {
		t.Fatal(err)
	}
	state.Documents[0].Answers["publish"] = form.Answer{Value: true, Human: true, Author: "Human", Revision: state.Revision, Round: 1}
	cfg := &config.Config{CleanupOnApprove: true}
	for _, approved := range []bool{false, true} {
		result := state.Result(approved, 1)
		srv := &tuiServer{cfg: cfg, sess: sess, mode: &reviewMode{docPath: "target.md"}}
		srv.handleFinish(approved, &result)
		payload := srv.lastPayload
		if payload.Result == nil || !strings.Contains(payload.Prompt, "Only consents explicitly true") || !strings.Contains(payload.Prompt, state.Revision) {
			t.Fatalf("missing mandatory contract: %+v", payload)
		}
		if !approved && len(payload.Result.Consents) != 0 {
			t.Fatal("feedback finalized consent")
		}
	}
	cleanupOnApprove(cfg, sess)
	if _, err := os.Stat(filepath.Join(sess.Dir, "forms.json")); err != nil {
		t.Fatalf("form approval deleted results: %v", err)
	}
}

func TestFormExportFailurePreservesApprovalData(t *testing.T) {
	old := resultPath
	t.Cleanup(func() { resultPath = old })
	resultPath = filepath.Join(t.TempDir(), "missing", "result.json")
	sess, err := review.OpenSession(t.TempDir(), "export")
	if err != nil {
		t.Fatal(err)
	}
	doc := review.AttachedDocument{ID: "a", Source: "/form.md", Name: "form.md", Content: "- [ ] Test <!-- form: id=test -->"}
	payload := &ipc.FinishPayload{Approved: true, Result: &review.FormResult{FormState: review.FormState{Version: 1, Revision: "exact", Documents: []review.FormDocument{{AttachedDocument: doc}}}, Approved: true}}
	if err := printFinish(payload, sess, nil); err == nil {
		t.Fatal("failed export succeeded")
	}
	if payload.Result.Revision != "exact" || !payload.Result.Approved {
		t.Fatal("failed export lost result")
	}
	resultPath = filepath.Join(t.TempDir(), "result.json")
	if err := printFinish(payload, sess, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil || !strings.Contains(string(data), `"review_revision": "exact"`) {
		t.Fatalf("missing exported snapshot: %s %v", data, err)
	}
}
