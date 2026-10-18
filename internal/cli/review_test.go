package cli

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
)

func TestShellEscape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"simple", "'simple'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
	}
	for _, tt := range tests {
		if got := shellEscape(tt.in); got != tt.want {
			t.Errorf("shellEscape(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveStagedReviewUsesIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Chdir(dir)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile("base.txt", []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "base.txt")
	run("commit", "-q", "-m", "initial")
	if err := os.WriteFile("staged.txt", []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "staged.txt")

	originalCode, originalStaged := reviewCode, reviewStaged
	reviewCode, reviewStaged = true, true
	t.Cleanup(func() {
		reviewCode, reviewStaged = originalCode, originalStaged
	})

	mode, err := resolveReviewMode(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !mode.staged || mode.ref != "HEAD" {
		t.Errorf("mode = %+v, want staged HEAD review", mode)
	}
	if len(mode.files) != 1 || mode.files[0].Path != "staged.txt" {
		t.Errorf("files = %+v, want only staged.txt", mode.files)
	}
}

func TestReviewModePersistedCLIArgs(t *testing.T) {
	tests := []struct {
		name string
		mode reviewMode
		want []string
	}{
		{name: "working tree", mode: reviewMode{ref: "HEAD"}},
		{name: "staged", mode: reviewMode{ref: "HEAD", staged: true}, want: []string{"--staged"}},
		{name: "document", mode: reviewMode{docPath: "plan.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.persistedCLIArgs(); !slices.Equal(got, tt.want) {
				t.Errorf("persistedCLIArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenReviewSessionPersistsStagedScope(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	cfg := &config.Config{Output: t.TempDir()}
	mode := &reviewMode{
		ref:    "HEAD",
		staged: true,
		files:  []git.FileChange{{Path: "staged.go", Status: git.StatusModified}},
	}

	sess, err := openReviewSession(cfg, mode)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := review.OpenSessionAt(sess.Key, sess.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CJ.BaseRef != "HEAD" {
		t.Errorf("base_ref = %q, want HEAD", reloaded.CJ.BaseRef)
	}
	if !slices.Equal(reloaded.CJ.CliArgs, []string{"--staged"}) {
		t.Errorf("cli_args = %q, want [--staged]", reloaded.CJ.CliArgs)
	}
	if reloaded.CJ.ActiveDiffScope != "" {
		t.Errorf("active_diff_scope = %q, want empty for a working-tree scope", reloaded.CJ.ActiveDiffScope)
	}
}

func TestTMUXDetectorUsesEnvironment(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux/default,100,2")
	t.Setenv("TMUX_PANE", "%7")

	context := (tmuxDetector{}).environmentContext()
	got, ok := context.(tmuxContext)
	if !ok {
		t.Fatalf("environmentContext() = %T, want tmuxContext", context)
	}
	if got.session != "/tmp/tmux/default,100,2" || got.pane != "%7" {
		t.Errorf("environmentContext() = %+v", got)
	}
}

func TestResolveTMUXContextFromAncestor(t *testing.T) {
	origOutput, origParent, origInspect, origLook := commandOutput, parentProcessID, inspectProcess, lookPath
	t.Cleanup(func() {
		commandOutput, parentProcessID, inspectProcess, lookPath = origOutput, origParent, origInspect, origLook
	})
	parentProcessID = func() int { return 400 }
	inspectProcess = func(pid int) (int, error) {
		if pid != 400 {
			return 0, fmt.Errorf("unexpected PID: %d", pid)
		}
		return 300, nil
	}
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	for _, tt := range []struct {
		name, panes, clients string
		want                 tmuxContext
	}{
		{"pane PID", "300\t/tmp/tmux-501/default\t100\t$2\t%7\n", "200\t/tmp/tmux-501/default\t100\t$3\n", tmuxContext{session: "/tmp/tmux-501/default,100,2", pane: "%7"}},
		{"client PID", "", "300\t/tmp/tmux-501/default\t100\t$3\n", tmuxContext{session: "/tmp/tmux-501/default,100,3"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				switch cmd.Args[1] {
				case "list-panes":
					if tt.panes == "" {
						return nil, fmt.Errorf("no panes")
					}
					return []byte(tt.panes), nil
				case "list-clients":
					return []byte(tt.clients), nil
				}
				return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
			}
			got, ok := walkAncestorContexts([]multiplexerDetector{tmuxDetector{}}).(tmuxContext)
			if !ok || got.session != tt.want.session || got.pane != tt.want.pane {
				t.Errorf("walkAncestorContexts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInspectParentProcess(t *testing.T) {
	got, err := inspectParentProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if want := os.Getppid(); got != want {
		t.Errorf("inspectParentProcess(%d) = %d, want %d", os.Getpid(), got, want)
	}
}

func TestTMUXCommandRestoresSessionEnvironment(t *testing.T) {
	t.Setenv("TMUX", "")
	cmd := tmuxCommand("/usr/bin/tmux", tmuxContext{session: "/tmp/tmux/default,100,2"}, "list-panes")

	var values []string
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "TMUX=") {
			values = append(values, entry)
		}
	}
	if want := []string{"TMUX=/tmp/tmux/default,100,2"}; !slices.Equal(values, want) {
		t.Errorf("TMUX environment = %v, want %v", values, want)
	}
}

func newPayloadSession(t *testing.T) *review.Session {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	sess, err := review.OpenSession("", "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestBuildFinishPayloadUnresolved(t *testing.T) {
	sess := newPayloadSession(t)
	sess.SetFileComments("a.go", "", []review.Comment{
		{ID: "c_1", StartLine: 3, EndLine: 3, Body: "fix this"},
		{ID: "c_2", StartLine: 9, EndLine: 9, Body: "done already", Resolved: true},
	})
	cfg := &config.Config{}

	payload := buildFinishPayload(cfg, sess, &reviewMode{ref: "HEAD", sessionKey: "0123456789ab"}, false)

	if payload.Approved {
		t.Error("expected unapproved payload")
	}
	if len(payload.Comments) != 2 {
		t.Errorf("expected all comments, got %+v", payload.Comments)
	}
	if want := "critty --session " + sess.Key; payload.NextCommand != want {
		t.Errorf("NextCommand = %q, want %q", payload.NextCommand, want)
	}
	for _, want := range []string{
		"The review finished with 1 unresolved comment.",
		`"id": "c_1"`,
		`"id": "c_2"`,
		"critty comment --session " + sess.Key + " --reply-to <comment-id>",
		payload.NextCommand,
	} {
		if !strings.Contains(payload.Prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, payload.Prompt)
		}
	}
}

func TestBuildFinishPayloadApproved(t *testing.T) {
	sess := newPayloadSession(t)
	cfg := &config.Config{}

	payload := buildFinishPayload(cfg, sess, &reviewMode{docPath: "doc.md"}, true)

	if !payload.Approved || len(payload.Comments) != 0 || payload.NextCommand != "" {
		t.Errorf("unexpected payload: %+v", payload)
	}
	if payload.Prompt != "Review approved with no comments — no changes requested.\n\nReview verdict: approve." {
		t.Errorf("prompt = %q", payload.Prompt)
	}

	sess.SetFileComments("a.go", "", []review.Comment{
		{ID: "c_1", StartLine: 1, EndLine: 1, Body: "Final instruction", Resolved: true, Replies: []review.Reply{{ID: "rp_1", Body: "Final reply"}}},
	})
	payload = buildFinishPayload(cfg, sess, &reviewMode{docPath: "doc.md"}, true)
	if len(payload.Comments) != 1 || !payload.Comments[0].Resolved || len(payload.Comments[0].Replies) != 1 {
		t.Fatalf("missing resolved thread: %+v", payload.Comments)
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	cleanupOnApprove(cfg, sess)
	retained, err := review.OpenSession("", sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	if comments := retained.FileComments("a.go"); len(comments) != 1 || len(comments[0].Replies) != 1 {
		t.Fatalf("default approval cleanup lost saved threads: %+v", comments)
	}
	cleanupOnApprove(&config.Config{CleanupOnApprove: true}, sess)
	if _, err := os.Stat(sess.Dir); !os.IsNotExist(err) {
		t.Fatalf("explicit approval cleanup retained the session: %v", err)
	}
	for _, want := range []string{"Final instruction", "Final reply", `"resolved": true`} {
		if !strings.Contains(payload.Prompt, want) {
			t.Errorf("approval output lost %q after cleanup: %s", want, payload.Prompt)
		}
	}
}

func TestBuildFinishPayloadPlanMode(t *testing.T) {
	sess := newPayloadSession(t)
	sess.CJ.CliArgs = []string{"plan", "--name", "my-plan", "docs/plan.md"}
	sess.SetFileComments("current.md", "", []review.Comment{
		{ID: "c_1", StartLine: 1, EndLine: 1, Body: "clarify"},
	})
	cfg := &config.Config{}
	mode := &reviewMode{docPath: "current.md", planSlug: "my-plan", planFile: "docs/plan.md"}

	payload := buildFinishPayload(cfg, sess, mode, false)

	if want := "critty plan --session " + sess.Key + " 'docs/plan.md'"; payload.NextCommand != want {
		t.Errorf("NextCommand = %q, want %q", payload.NextCommand, want)
	}
	for _, want := range []string{
		"critty comment --session " + sess.Key + " --reply-to <comment-id>",
		payload.NextCommand,
	} {
		if !strings.Contains(payload.Prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, payload.Prompt)
		}
	}
}

func TestReviewArgSuffixEscapesDocumentPath(t *testing.T) {
	mode := &reviewMode{docPath: "plans/it's a plan.md"}
	if got, want := reviewArgSuffix(mode), " 'plans/it'\\''s a plan.md'"; got != want {
		t.Fatalf("reviewArgSuffix = %q, want %q", got, want)
	}
}

func TestOverallSubmissionSurvivesCustomPrompts(t *testing.T) {
	for _, verdict := range []review.Verdict{review.VerdictApprove, review.VerdictReject, review.VerdictComment} {
		t.Run(string(verdict), func(t *testing.T) {
			sess := newPayloadSession(t)
			sess.SetSubmission(verdict, "Overall instructions.", "Human")
			cfg := &config.Config{Prompts: map[string]string{
				"on_finish_approved":   "inline:Custom approval for {{.session_key}}",
				"on_finish_unresolved": "inline:Custom feedback for {{.session_key}}",
			}}
			payload := buildFinishPayload(cfg, sess, &reviewMode{docPath: "doc.md"}, verdict == review.VerdictApprove)
			if payload.Verdict != verdict || payload.ReviewComment == nil || payload.ReviewComment.Body != "Overall instructions." {
				t.Fatalf("submission missing: %+v", payload)
			}
			if !strings.HasPrefix(payload.Prompt, "Custom ") || !strings.Contains(payload.Prompt, " for "+sess.Key) || !strings.Contains(payload.Prompt, "Overall instructions.") || !strings.Contains(payload.Prompt, "Review verdict: "+string(verdict)) {
				t.Fatalf("custom prompt hid the submission: %s", payload.Prompt)
			}
			if (payload.NextCommand == "") != (verdict == review.VerdictApprove) {
				t.Fatalf("wrong continuation: %+v", payload)
			}
		})
	}
}
