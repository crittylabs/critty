package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/document"
	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

var reviewCode bool
var reviewStaged bool
var reviewUnstaged bool
var reviewDiff string
var reviewNull bool
var reviewScope string

// The following function variables allow tests to replace shell interactions
// without actually shelling out.
var runCommand = func(cmd *exec.Cmd) error {
	return cmd.Run()
}

var lookPath = exec.LookPath

var commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

var parentProcessID = os.Getppid

var inspectProcess = inspectParentProcess

var resolveExec = func() (string, error) {
	return resolveExecutable()
}

var reviewCmd = &cobra.Command{
	Use:   "review [file[:LINE]]",
	Short: "Review git changes (default) or a single document",
	Long: `Open a review and block until the human finishes it.

With no file argument, reviews the current git changes (multi-file mode).
Git reviews apply diff.<driver>.textconv only for driver names or glob patterns
enabled in Critty's textconv_drivers setting.  Review lines then refer to the
converted UTF-8 text.
With a file argument, reviews that document.
With --diff[=FILE], reads a Git unified diff from a file or stdin (-), without
requiring a repository.  A file argument after --diff is also accepted.
With --null, reviews only attachments without requiring a repository.
Use critty view for a read-only view without saving.

Inside a supported multiplexer the TUI opens in a dedicated pane or tab.
Outside a multiplexer it opens a new desktop terminal window when one is
available, and otherwise runs in the current terminal. Use --launcher to
change the order or select another launcher, such as mux or
custom:EXECUTABLE; see its flag help for the full list. This command blocks
until the reviewer approves or finishes with comments, printing the
resulting agent prompt on stdout and "approved: true|false" on stderr.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReview(args)
	},
}

// reviewMode describes what is being reviewed.
type reviewMode struct {
	documents         []review.AttachedDocument
	replaceDocuments  bool
	documentsPrepared bool
	docPath           string // non-empty for single-document and plan modes
	ref               string // diff base for code mode
	files             []git.FileChange
	staged            bool
	planSlug          string // non-empty for plan mode
	planFile          string // original plan path ("" when read from stdin)
	planContent       []byte // replacement plan input, saved after acquiring the run lock
	patch             *git.Patch
	sessionKey        string // exact saved session opened by the TUI
	source            *git.ReviewSource
	focusPath         string
	focusLine         int
}

func (m *reviewMode) code() bool { return m.docPath == "" }

func (m *reviewMode) plan() bool { return m.planSlug != "" }

func (m *reviewMode) promptMode() string {
	if m.code() {
		return "diff"
	}
	return "files"
}

func (m *reviewMode) internalMode() string {
	switch {
	case m.patch != nil:
		return "diff"
	case m.plan():
		return "plan"
	case m.code():
		return "git"
	default:
		return "files"
	}
}

// persistedCLIArgs returns the code-review scope arguments recorded in
// review.json.  BaseRef carries the resolved diff base; CliArgs distinguishes
// staged reviews, whose base is also HEAD, from full working-tree reviews.
func (m *reviewMode) persistedCLIArgs() []string {
	if !m.code() {
		return nil
	}
	if m.patch != nil {
		if len(m.patch.Files) == 0 {
			return []string{"--null"}
		}
		return []string{"--diff"}
	}
	if m.source != nil {
		if m.source.Scope == "range" {
			return []string{"--scope", m.source.Range}
		}
		return []string{"--scope", m.source.Scope}
	}
	if m.staged {
		return []string{"--staged"}
	}
	return nil
}

func runReview(args []string) error {
	return runReviewInput(args, false)
}

func runReviewInput(args []string, viewOnly bool) error {
	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}

	mode, err := resolveReviewMode(args)
	if err != nil {
		return err
	}
	if err := prepareDocuments(mode); err != nil {
		return err
	}
	if err := mode.validateTarget(); err != nil {
		return err
	}

	if reviewDemo || viewOnly {
		return runTransientReview(cfg, mode)
	}

	sess, err := openReviewSession(cfg, mode)
	if err != nil {
		return err
	}

	return runReviewFlow(cfg, sess, mode)
}

// runReviewFlow opens one round in the caller's multiplexer or terminal.
func runReviewFlow(cfg *config.Config, sess *review.Session, mode *reviewMode) (result error) {
	if err := prepareDocuments(mode); err != nil {
		return err
	}
	if err := mode.validateTarget(); err != nil {
		return err
	}
	if err := validateResultPath(mode, sess); err != nil {
		return err
	}
	if err := validateSummaryPath(mode, sess); err != nil {
		return err
	}
	if err := resolveFocus(mode); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(sess.Dir, "run.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("review %s is already active", sess.Key)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			fmt.Fprintf(os.Stderr, "critty: releasing review lock: %v\n", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "Review session: %s\n", sess.Key)
	sock := review.SocketPathFor(sess.Key)
	if ipc.Alive(sock) {
		return fmt.Errorf("review %s is already active; stop it before resuming", sess.Key)
	}
	applyHeaderFlags(sess)
	if err := saveReviewMode(sess, mode); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	store := history.New(cwd)
	if reviewTask != "" {
		if err := store.Add(reviewTask, sess.Key); err != nil {
			return fmt.Errorf("review %s was saved but could not join task %s: %w; retry with history add before resuming this review", sess.Key, reviewTask, err)
		}
	}
	cfg, err = taskReviewConfig(cfg, sess)
	if err != nil {
		return err
	}

	warnHistory(os.Stderr, sess.Meta.CWD, cfg.History)

	multiplexer, err := reviewTerminalContext()
	if err != nil {
		return err
	}
	launcher, err := selectReviewLauncher(slices.Concat(reviewLauncherNames, cfg.Launchers), multiplexer, mode)
	if err != nil && (!errors.Is(err, errNoReviewLauncher) || multiplexer != nil) {
		return err
	}
	if launcher != nil {
		launch, err := launcher.open(mode)
		if err != nil {
			return err
		}
		defer launch.restoreFocus()
		defer func() { result = errors.Join(result, launch.close()) }()
		if err := ipc.WaitAlive(sock, 15*time.Second); err != nil {
			return err
		}
		return runReviewCycle(cfg, sess, sock)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) || mode.patch != nil {
		payload, err := runTUISession(cfg, sess, mode, false)
		if err != nil {
			return err
		}
		if payload == nil {
			fmt.Fprintln(os.Stderr, "approved: false")
			return fmt.Errorf("review ended without finishing")
		}
		if err := printFinish(payload, sess); err != nil {
			return err
		}
		if payload.Approved {
			cleanupOnApprove(cfg, sess)
		}
		return nil
	}

	return withCodexWrapperHint(fmt.Errorf("%w; no terminal to open the TUI in; run inside a supported multiplexer, or have the reviewer run `critty%s` in a terminal", err, reviewArgSuffix(mode)))
}

func reviewArgSuffix(mode *reviewMode) string {
	if mode.source != nil {
		args := mode.persistedCLIArgs()
		return " " + args[0] + " " + shellEscape(args[1])
	}
	if mode.code() {
		if mode.staged {
			return " --staged"
		}
		return ""
	}
	return " " + shellEscape(mode.docPath)
}

// resolveReviewMode classifies the arguments and, for code mode, detects
// the changed files up front so failures surface before any TUI spawns.
func resolveReviewMode(args []string) (*reviewMode, error) {
	if reviewNull {
		if reviewDiff != "" || reviewScope != "" || reviewStaged || reviewUnstaged || reviewCode || len(args) > 0 {
			return nil, fmt.Errorf("--null cannot be combined with --diff, --scope, --code, --staged, --unstaged, or a file argument")
		}
		return &reviewMode{patch: &git.Patch{}}, nil
	}
	if reviewDiff != "" {
		if reviewScope != "" || reviewStaged || reviewUnstaged || reviewCode {
			return nil, fmt.Errorf("--diff cannot be combined with --scope, --code, --staged, or --unstaged")
		}
		input := reviewDiff
		if len(args) > 0 {
			if input != "-" {
				return nil, fmt.Errorf("--diff accepts only one input file")
			}
			input = args[0]
		}
		patch, err := readDiff(input)
		if err != nil {
			return nil, err
		}
		return &reviewMode{patch: patch, files: patch.Changes()}, nil
	}
	if (reviewStaged || reviewUnstaged || reviewScope != "") && len(args) > 0 {
		return nil, fmt.Errorf("--scope, --staged, and --unstaged are only valid for code review")
	}
	if len(args) == 1 && !reviewCode {
		filePath, line, err := parseFocus(args[0])
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		mode := &reviewMode{docPath: filePath}
		if line > 0 {
			doc, err := document.Load(filePath)
			if err != nil {
				return nil, err
			}
			if !doc.HasLine(line) {
				return nil, fmt.Errorf("focus line %d is not in the review: %s", line, filePath)
			}
			mode.focusPath, mode.focusLine = filePath, line
		}
		return mode, nil
	}

	if !git.IsGitRepo() {
		return nil, fmt.Errorf("code review requires a git repository (pass a file argument to review a document)")
	}

	return resolveCodeScope()
}

func (m *reviewMode) validateTarget() error {
	if m.patch == nil || len(m.files) > 0 {
		return nil
	}
	for _, d := range m.documents {
		if !d.Removed {
			return nil
		}
	}
	return fmt.Errorf("empty diff requires --attachment")
}

func openReviewSession(cfg *config.Config, mode *reviewMode) (*review.Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	args := mode.persistedCLIArgs()
	if !mode.code() {
		args = []string{review.NormalizePath(mode.docPath)}
	}
	branch, _ := git.CurrentBranch()
	sess, err := review.NewSession(cfg.Output, review.SessionEntry{CWD: cwd, Branch: branch, Args: args, Mode: mode.internalMode()})
	if err != nil {
		return nil, err
	}
	if err := saveReviewMode(sess, mode); err != nil {
		return nil, err
	}
	return sess, nil
}

func saveReviewMode(sess *review.Session, mode *reviewMode) error {
	mode.sessionKey = sess.Key
	if mode.replaceDocuments {
		if err := sess.ReplaceDocuments(mode.documents); err != nil {
			return err
		}
		mode.replaceDocuments = false
	}
	if answersMode || noAnswersMode {
		if err := sess.SetAnswerMode(answersMode); err != nil {
			return err
		}
	}
	if mode.planContent != nil {
		if _, err := review.SavePlanVersionAt(filepath.Dir(mode.docPath), mode.planContent); err != nil {
			return err
		}
		mode.planContent = nil
	}
	if mode.patch != nil {
		if err := sess.SaveDiff(mode.patch); err != nil {
			return err
		}
	}
	return sess.Update(func(s *review.Session) error {
		s.CJ.BaseRef = mode.ref
		s.CJ.CliArgs = s.Meta.Args
		s.CJ.Branch = s.Meta.Branch
		if mode.plan() {
			s.CJ.CliArgs = []string{"plan", "--name", mode.planSlug, mode.planFile}
		}
		for _, f := range mode.files {
			s.SetFileComments(f.Path, f.ReviewStatus(), s.FileComments(f.Path))
		}
		return nil
	})
}

// runReviewCycle blocks on the session socket until the reviewer finishes,
// then prints the agent-facing result and applies approval cleanup.
func runReviewCycle(cfg *config.Config, sess *review.Session, sock string) error {
	payload, err := ipc.ReviewCycle(sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "approved: false")
		return err
	}
	if err := printFinish(payload, sess); err != nil {
		return err
	}
	if payload.Approved {
		cleanupOnApprove(cfg, sess)
	}
	return nil
}

// printFinish writes the agent contract: the rendered prompt on stdout and
// the approval verdict on stderr.
func printFinish(payload *ipc.FinishPayload, sess *review.Session) error {
	if resultPath != "" {
		if payload.Result == nil {
			return fmt.Errorf("--result requires an interactive attachment form")
		}
		mode := &reviewMode{}
		for _, d := range payload.Result.Documents {
			mode.documents = append(mode.documents, d.AttachedDocument)
		}
		for path := range payload.Result.Input.Files {
			mode.files = append(mode.files, git.FileChange{Path: path})
		}
		if err := validateResultPath(mode, sess); err != nil {
			return err
		}
		if err := review.WriteJSON(resultPath, payload.Result); err != nil {
			return fmt.Errorf("saving form result: %w", err)
		}
	}
	if summaryPath != "" {
		if err := writeFinishSummary(payload, sess); err != nil {
			return fmt.Errorf("saving finish summary: %w", err)
		}
	}
	if payload.Prompt != "" {
		if _, err := fmt.Fprintln(os.Stdout, payload.Prompt); err != nil {
			return err
		}
	}
	if payload.Verdict.Valid() {
		if _, err := fmt.Fprintf(os.Stderr, "verdict: %s\n", payload.Verdict); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(os.Stderr, "approved: %t\n", payload.Approved)
	return err
}

func cleanupOnApprove(cfg *config.Config, sess *review.Session) {
	if !cfg.CleanupOnApprove || sess.HasAttachments() {
		return
	}
	forms, err := sess.LoadForms()
	if err != nil || forms.HasForms() {
		return
	}
	if err := sess.Clear(); err != nil {
		fmt.Fprintf(os.Stderr, "critty: warning: could not clean up review: %v\n", err)
	}
}

// buildTUICommand builds the shell command that runs the TUI inside a pane
// of host, the multiplexer named in the TUI's top bar.
func buildTUICommand(mode *reviewMode, host string) (string, error) {
	crittyBin, err := resolveExec()
	if err != nil {
		return "", fmt.Errorf("resolving critty binary path: %w", err)
	}

	// Multiplexer panes inherit their host's environment, not necessarily the
	// caller's, so pass through the variables that locate review state.
	envPrefix := "env CRITTY_HOST=" + shellEscape(host)
	for _, name := range []string{"XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		if val := os.Getenv(name); val != "" {
			envPrefix += " " + name + "=" + shellEscape(val)
		}
	}

	if mode.sessionKey == "" {
		return "", fmt.Errorf("missing review session ID")
	}
	command := fmt.Sprintf("%s %s _tui --session %s", envPrefix, shellEscape(crittyBin), shellEscape(mode.sessionKey))
	if mode.focusPath != "" {
		focus := mode.focusPath
		if mode.focusLine > 0 {
			focus += ":" + strconv.Itoa(mode.focusLine)
		}
		command += " --focus " + shellEscape(focus)
	}
	return command, nil
}

// resolveExecutable returns the absolute path to the currently running binary.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// shellEscape escapes a string for safe embedding in a POSIX shell command.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func init() {
	reviewCmd.PreRunE = validateScopeFlags
	reviewCmd.Flags().StringVar(&reviewScope, "scope", "", "review scope: all (default), staged, unstaged, A..B, or A...B (omitted B means HEAD)")
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().BoolVar(&reviewCode, "code", false, "review code changes (default when no file argument is given)")
	addDiffFlag(reviewCmd)
	addFocusFlag(reviewCmd)
	addHeaderFlags(reviewCmd)
	addAttachmentFlags(reviewCmd)
	reviewCmd.Flags().BoolVar(&reviewStaged, "staged", false, "review only changes staged in the index (alias for --scope=staged)")
	reviewCmd.Flags().BoolVar(&reviewUnstaged, "unstaged", false, "review unstaged and untracked changes (alias for --scope=unstaged)")

	// Deprecated no-ops: blocking on a tmux split pane is now the default.
	var deprecatedDetach, deprecatedWait bool
	reviewCmd.Flags().BoolVar(&deprecatedDetach, "detach", false, "deprecated: no-op")
	reviewCmd.Flags().BoolVar(&deprecatedWait, "wait", false, "deprecated: no-op")
	for _, name := range []string{"detach", "wait"} {
		if err := reviewCmd.Flags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
}
