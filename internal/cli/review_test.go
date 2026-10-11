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

func TestRootCommandSupportsStagedFlag(t *testing.T) {
	if rootCmd.Flags().Lookup("staged") == nil {
		t.Fatal("root command has no --staged flag")
	}
}

func TestSplitWindowArgsTargetsInvokingPaneAndCwd(t *testing.T) {
	args := splitWindowArgs(true, "cmd", "%42")

	idx := slices.Index(args, "-t")
	if idx < 0 || args[idx+1] != "%42" {
		t.Errorf("expected -t %%42 in %v", args)
	}
	cwd, _ := os.Getwd()
	cIdx := slices.Index(args, "-c")
	if cIdx < 0 || args[cIdx+1] != cwd {
		t.Errorf("expected -c %s in %v", cwd, args)
	}
	if !slices.Contains(args, "-p") {
		t.Errorf("expected -p in %v", args)
	}
	if args[len(args)-1] != "cmd" {
		t.Errorf("expected command last in %v", args)
	}
}

func TestSplitWindowArgsWithoutSize(t *testing.T) {
	args := splitWindowArgs(false, "cmd", "")
	if slices.Contains(args, "-p") {
		t.Errorf("expected no -p in %v", args)
	}
	if slices.Contains(args, "-t") {
		t.Errorf("expected no -t without TMUX_PANE in %v", args)
	}
}

// captureSpawns replaces the shell seams and records tmux invocations.
func captureSpawns(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	origRun, origOutput, origLook, origResolve := runCommand, commandOutput, lookPath, resolveExec
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		calls = append(calls, cmd.Args)
		return []byte("%99\n"), nil
	}
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Cleanup(func() {
		runCommand, commandOutput, lookPath, resolveExec = origRun, origOutput, origLook, origResolve
	})
	return &calls
}

func TestSpawnTUIPaneCodeMode(t *testing.T) {
	calls := captureSpawns(t)

	mode := &reviewMode{ref: "HEAD", sessionKey: "0123456789ab"}
	if _, err := spawnTUIPane(mode, tmuxContext{session: "/tmp/tmux/default,100,1", pane: "%42"}); err != nil {
		t.Fatalf("spawnTUIPane: %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(*calls))
	}
	cmd := (*calls)[0][len((*calls)[0])-1]
	for _, want := range []string{"TCRIT_HOST='tmux'", "_tui", "--session"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("pane command missing %q: %s", want, cmd)
		}
	}
	if !slices.Contains((*calls)[0], "%42") {
		t.Errorf("tmux call should target the resolved pane: %v", (*calls)[0])
	}
}

func TestBuildTUICommandStagedMode(t *testing.T) {
	captureSpawns(t)

	cmd, err := buildTUICommand(&reviewMode{ref: "HEAD", staged: true, sessionKey: "0123456789ab"}, "tmux")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd, "_tui --session") {
		t.Errorf("pane command missing staged scope: %s", cmd)
	}
	if strings.Contains(cmd, "--base") {
		t.Errorf("staged pane command should not include --base: %s", cmd)
	}
}

func TestSpawnTUIPaneDocMode(t *testing.T) {
	calls := captureSpawns(t)

	mode := &reviewMode{docPath: "docs/it's plan.md", sessionKey: "0123456789ab"}
	if _, err := spawnTUIPane(mode, tmuxContext{}); err != nil {
		t.Fatalf("spawnTUIPane: %v", err)
	}

	cmd := (*calls)[0][len((*calls)[0])-1]
	if !strings.Contains(cmd, "--session '0123456789ab'") {
		t.Errorf("pane command should escape quotes: %s", cmd)
	}
	if strings.Contains(cmd, "--base") {
		t.Errorf("doc mode should not pass --base: %s", cmd)
	}
}

func TestSpawnTUIPaneRetriesWithoutPercentage(t *testing.T) {
	var calls [][]string
	origRun, origOutput, origLook, origResolve := runCommand, commandOutput, lookPath, resolveExec
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		calls = append(calls, cmd.Args)
		if len(calls) == 1 {
			return nil, fmt.Errorf("size unavailable")
		}
		return []byte("%99\n"), nil
	}
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Cleanup(func() {
		runCommand, commandOutput, lookPath, resolveExec = origRun, origOutput, origLook, origResolve
	})

	if _, err := spawnTUIPane(&reviewMode{ref: "HEAD", sessionKey: "0123456789ab"}, tmuxContext{}); err != nil {
		t.Fatalf("spawnTUIPane: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected retry, got %d calls", len(calls))
	}
	if !slices.Contains(calls[0], "-p") {
		t.Errorf("first attempt should include -p: %v", calls[0])
	}
	if slices.Contains(calls[1], "-p") {
		t.Errorf("retry should omit -p: %v", calls[1])
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

func TestResolveTMUXContextFromAncestorPanePID(t *testing.T) {
	origOutput, origParent, origInspect, origLook := commandOutput, parentProcessID, inspectProcess, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		switch cmd.Args[1] {
		case "list-panes":
			return []byte("300\t/tmp/tmux-501/default\t100\t$2\t%7\n"), nil
		case "list-clients":
			return []byte("200\t/tmp/tmux-501/default\t100\t$3\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		}
	}
	parentProcessID = func() int { return 400 }
	inspectProcess = func(pid int) (int, error) {
		switch pid {
		case 400:
			return 300, nil
		default:
			return 0, fmt.Errorf("unexpected PID: %d", pid)
		}
	}
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	t.Cleanup(func() {
		commandOutput, parentProcessID, inspectProcess, lookPath = origOutput, origParent, origInspect, origLook
	})

	context := walkAncestorContexts([]multiplexerDetector{tmuxDetector{}})
	got, ok := context.(tmuxContext)
	if !ok {
		t.Fatalf("walkAncestorContexts() = %T, want tmuxContext", context)
	}
	if got.session != "/tmp/tmux-501/default,100,2" || got.pane != "%7" {
		t.Errorf("walkAncestorContexts() = %+v", got)
	}
}

func TestResolveTMUXContextFallsBackToClientPID(t *testing.T) {
	origOutput, origParent, origInspect, origLook := commandOutput, parentProcessID, inspectProcess, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[1] == "list-clients" {
			return []byte("300\t/tmp/tmux-501/default\t100\t$3\n"), nil
		}
		return nil, fmt.Errorf("no panes")
	}
	parentProcessID = func() int { return 400 }
	inspectProcess = func(int) (int, error) { return 300, nil }
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	t.Cleanup(func() {
		commandOutput, parentProcessID, inspectProcess, lookPath = origOutput, origParent, origInspect, origLook
	})

	context := walkAncestorContexts([]multiplexerDetector{tmuxDetector{}})
	got, ok := context.(tmuxContext)
	if !ok {
		t.Fatalf("walkAncestorContexts() = %T, want tmuxContext", context)
	}
	if got.session != "/tmp/tmux-501/default,100,3" || got.pane != "" {
		t.Errorf("walkAncestorContexts() = %+v", got)
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
	if payload.Prompt != "Review approved with no comments — no changes requested." {
		t.Errorf("prompt = %q", payload.Prompt)
	}

	sess.SetFileComments("a.go", "", []review.Comment{
		{ID: "c_1", StartLine: 1, EndLine: 1, Body: "Final instruction", Resolved: true, Replies: []review.Reply{{ID: "rp_1", Body: "Final reply"}}},
	})
	payload = buildFinishPayload(cfg, sess, &reviewMode{docPath: "doc.md"}, true)
	if len(payload.Comments) != 1 || !payload.Comments[0].Resolved || len(payload.Comments[0].Replies) != 1 {
		t.Fatalf("missing resolved thread: %+v", payload.Comments)
	}
	cleanupOnApprove(&config.Config{CleanupOnApprove: true}, sess)
	for _, want := range []string{"Final instruction", "Final reply", `"resolved": true`} {
		if !strings.Contains(payload.Prompt, want) {
			t.Errorf("approval output lost %q after cleanup: %s", want, payload.Prompt)
		}
	}
}

func TestBuildFinishPayloadUsesConfigPromptOverride(t *testing.T) {
	sess := newPayloadSession(t)
	cfg := &config.Config{Prompts: map[string]string{
		"on_finish_approved": "inline:Custom approved for {{.session_key}}",
	}}

	payload := buildFinishPayload(cfg, sess, &reviewMode{docPath: "doc.md"}, true)
	if want := "Custom approved for " + sess.Key; payload.Prompt != want {
		t.Errorf("prompt = %q, want %q", payload.Prompt, want)
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
