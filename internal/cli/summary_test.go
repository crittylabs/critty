package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

func captureFinish(t *testing.T, payload *ipc.FinishPayload, sess *review.Session, launch *launchSummary) (string, string, error) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := out.Close(); err != nil {
			t.Error(err)
		}
	}()
	errOut, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := errOut.Close(); err != nil {
			t.Error(err)
		}
	}()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errOut
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	finishErr := printFinish(payload, sess, launch)
	outData, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	errData, err := os.ReadFile(errOut.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(outData), string(errData), finishErr
}

func useSummaryPath(t *testing.T, path string) {
	t.Helper()
	oldSummary, oldResult := summaryPath, resultPath
	summaryPath, resultPath = path, ""
	t.Cleanup(func() { summaryPath, resultPath = oldSummary, oldResult })
}

func TestFinishSummaryPreservesAllFeedbackWithoutAttachmentBodies(t *testing.T) {
	for _, verdict := range []review.Verdict{review.VerdictApprove, review.VerdictComment, review.VerdictReject} {
		t.Run(string(verdict), func(t *testing.T) {
			sess := newPayloadSession(t)
			sess.CJ.ReviewRound = 3
			sess.SetSubmission(verdict, "Overall instructions", "Human")
			sess.SetFileComments("file.md", "modified", []review.Comment{{
				ID: "c_resolved", Body: "![Context](attachments/context.png)", Resolved: true,
				Replies: []review.Reply{{ID: "rp_1", Body: "Already addressed"}},
			}})
			input := filepath.Join(t.TempDir(), "notes with spaces.md")
			body := "Large attachment body\n" + strings.Repeat("Review approved.\n", 1024)
			if err := os.WriteFile(input, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			docs, err := review.ReadAttachedDocuments([]string{input})
			if err != nil {
				t.Fatal(err)
			}
			docs[0].Removed = true
			if err := sess.ReplaceDocuments(docs); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Prompts: map[string]string{
				"on_finish_approved":   "inline:Custom approved heading",
				"on_finish_unresolved": "inline:Custom feedback heading",
			}}
			payload := buildFinishPayload(cfg, sess, &reviewMode{docPath: "file.md"}, verdict == review.VerdictApprove)
			useSummaryPath(t, filepath.Join(t.TempDir(), "summary.json"))
			launch := &launchSummary{
				Launcher:      "custom:review",
				Failures:      []launcherFailure{{Launcher: "mux", Reason: "no terminal"}},
				TerminalError: "marker not found",
			}
			out, stderr, err := captureFinish(t, &payload, sess, launch)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, body) || !strings.Contains(stderr, "verdict: "+string(verdict)) {
				t.Fatal("complete finish output changed")
			}
			data, err := os.ReadFile(summaryPath)
			if err != nil {
				t.Fatal(err)
			}
			var summary finishSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if summary.Version != 1 || summary.Session != sess.Key || summary.Round != 3 || summary.Verdict != verdict || summary.Approved != payload.Approved {
				t.Fatalf("wrong finish identity: %+v", summary)
			}
			if summary.Launch == nil || summary.Launch.Launcher != launch.Launcher || summary.Launch.TerminalError != launch.TerminalError || len(summary.Launch.Failures) != 1 || summary.Launch.Failures[0] != launch.Failures[0] {
				t.Fatalf("lost launcher diagnostics: %+v", summary.Launch)
			}
			if summary.ReviewComment == nil || len(summary.Comments) != 2 || !summary.Comments[1].Resolved || len(summary.Comments[1].Replies) != 1 {
				t.Fatalf("lost feedback: %+v", summary)
			}
			if !strings.Contains(summary.Comments[1].Body, "attachments/context.png") || summary.ImageBase != sess.Dir {
				t.Fatal("lost image reference or base")
			}
			if len(summary.Attachments) != 1 || summary.Attachments[0].Path != docs[0].Path() || summary.Attachments[0].Source != docs[0].Source || summary.Attachments[0].Revision != docs[0].Revision || !summary.Attachments[0].Removed {
				t.Fatalf("lost attachment identity: %+v", summary.Attachments)
			}
			if strings.Contains(string(data), "Large attachment body") || summary.NextCommand != payload.NextCommand {
				t.Fatal("summary copied a body or lost continuation")
			}
		})
	}
}

func TestFinishSummaryWithoutCommentsSurvivesCleanup(t *testing.T) {
	sess := newPayloadSession(t)
	payload := buildFinishPayload(&config.Config{}, sess, &reviewMode{docPath: "doc.md"}, true)
	useSummaryPath(t, filepath.Join(t.TempDir(), "summary.json"))
	if _, _, err := captureFinish(t, &payload, sess, &launchSummary{Launcher: "foreground", Failures: []launcherFailure{}}); err != nil {
		t.Fatal(err)
	}
	cleanupOnApprove(&config.Config{CleanupOnApprove: true}, sess)
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"comments": []`) || !strings.Contains(string(data), `"attachments": []`) || !strings.Contains(string(data), `"approved": true`) {
		t.Fatalf("missing empty-result contract: %s", data)
	}
	if !strings.Contains(string(data), `"failures": []`) || strings.Contains(string(data), `"terminal_error"`) {
		t.Fatalf("wrong empty launch diagnostics: %s", data)
	}
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Fatal("ordinary review was not cleaned up")
	}
}

func TestSummaryExportFailureRetainsOldFileAndPrintsNoApproval(t *testing.T) {
	sess := newPayloadSession(t)
	payload := buildFinishPayload(&config.Config{}, sess, &reviewMode{docPath: "doc.md"}, true)
	useSummaryPath(t, filepath.Join(t.TempDir(), "summary.json"))
	old := []byte(`{"approved":true,"round":0}`)
	if err := os.WriteFile(summaryPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sess.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sess.Dir, "documents.json"), []byte("truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := captureFinish(t, &payload, sess, nil)
	if err == nil || out != "" || strings.Contains(stderr, "approved: true") {
		t.Fatalf("export failure granted approval: %q %q %v", out, stderr, err)
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil || string(data) != string(old) {
		t.Fatal("failed export replaced previous evidence")
	}
	if _, err := os.Stat(sess.Dir); err != nil {
		t.Fatal("failed export deleted saved review")
	}
}

func TestSummaryPathProtectsReviewedInputAndSession(t *testing.T) {
	sess := newPayloadSession(t)
	input := filepath.Join(t.TempDir(), "input.md")
	if err := os.WriteFile(input, []byte("Original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sess.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(sess.Dir, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{input, filepath.Join(alias, "new-summary.json")} {
		useSummaryPath(t, path)
		if err := validateSummaryPath(&reviewMode{docPath: input}, sess); err == nil {
			t.Fatalf("unsafe summary destination accepted: %s", path)
		}
	}
	useSummaryPath(t, filepath.Join(t.TempDir(), "summary.json"))
	resultPath = summaryPath
	if err := validateSummaryPath(&reviewMode{}, sess); err == nil {
		t.Fatal("summary and form result may overwrite each other")
	}
}
