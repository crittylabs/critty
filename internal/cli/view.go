package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/v2/internal/config"
	gitpkg "github.com/crittylabs/critty/v2/internal/git"
	"github.com/crittylabs/critty/v2/internal/nerdfont"
	"github.com/crittylabs/critty/v2/internal/review"
	"github.com/crittylabs/critty/v2/internal/tui"
)

var reviewDemo bool

var viewCmd = &cobra.Command{
	Use:     "view [file[:LINE]]",
	Short:   "View changes or a document without reviewing or saving",
	Long:    "View Git changes, a document, a supplied diff, or attachments in the current terminal.  Follow local file and heading links, and use Back/Forward to restore reading positions.  Web links open in the browser; unsupported files offer to open externally.  Review editing is disabled, and nothing is saved.",
	Args:    cobra.MaximumNArgs(1),
	PreRunE: validateScopeFlags,
	RunE:    func(_ *cobra.Command, args []string) error { return runReviewInput(args, true) },
}

func defaultFileView(args []string) bool {
	// --diff FILE leaves FILE positional because --diff also accepts stdin.
	return rootSession == "" && reviewDiff == "" && !reviewCode && len(args) == 1
}

func transientReview(cmd *cobra.Command, args []string) bool {
	return cmd == viewCmd || reviewDemo || cmd == rootCmd && defaultFileView(args)
}

func validateTransientFlags(cmd *cobra.Command) error {
	mode := "view"
	if reviewDemo {
		mode = "--demo"
	}
	for _, name := range []string{"session", "task", "result", "summary", "terminal-request", "launcher", "clear-attachments"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("%s cannot be combined with --%s", mode, name)
		}
	}
	return nil
}

// runTransientReview opens the current terminal without storage or an IPC server.
func runTransientReview(cfg *config.Config, mode *reviewMode) (result error) {
	if err := resolveFocus(mode); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	project := reviewProjectName
	if !headerFlagSpecified("project") {
		project = reviewProject(cfg.ProjectRoot)
	}
	branch, _ := gitpkg.CurrentBranch()
	appCfg := tui.AppConfig{
		MaxWrapWidth:     configuredWrapWidth(cfg),
		LineNumbers:      cfg.LineNumbers,
		Syntax:           cfg.SyntaxRules,
		NerdFonts:        nerdfont.Detect(cfg.NerdFonts).Supported,
		MathRaTeX:        cfg.MathRaTeX,
		MermaidMerman:    cfg.MermaidMerman,
		MermaidMmdr:      cfg.MermaidMmdr,
		MermaidFonts:     cfg.MermaidFonts,
		RemoteImageHosts: cfg.RemoteImageHosts,
		Branch:           branch,
		Context:          ctx,
		Watch:            reviewWatch,
		Documents:        mode.documents,
		Project:          project,
		GitProject:       reviewGitProject(cfg.ProjectRoot),
		Topic:            reviewTopic,
		Version:          versionString(),
		ReadOnly:         !reviewDemo,
		SearchHistory:    searchHistoryStore(cfg.ProjectRoot, nil),
		Answers:          answersMode,
		Author:           cfg.Author,
		Split:            cfg.Split,
		Staged:           mode.staged,
		Source:           mode.source,
		Patch:            mode.patch,
		FocusPath:        mode.focusPath,
		FocusLine:        mode.focusLine,
	}
	if reviewDemo {
		appCfg.Session = review.NewMemorySession()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		appCfg.Session.Meta.CWD = cwd
		defer func() { result = errors.Join(result, appCfg.Session.Close()) }()
		if err := appCfg.Session.ReplaceDocuments(mode.documents); err != nil {
			return err
		}
	}
	var model tui.AppModel
	if mode.code() {
		model = tui.NewCodeReviewApp(mode.files, mode.ref, appCfg)
	} else {
		model = tui.NewApp(mode.docPath, appCfg)
	}
	if reviewWatch && !model.CanWatch() {
		return fmt.Errorf("--watch requires a live document, Git scope, or file-backed attachment")
	}
	// Stdin may contain a patch.  Always read interaction from the foreground TTY.
	if !tui.InteractiveTerminal() {
		return fmt.Errorf("this mode requires an interactive foreground terminal")
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("this mode requires a foreground terminal: %w", err)
	}
	defer tty.Close()
	options := append([]tea.ProgramOption{tea.WithInput(tty), tea.WithOutput(tty)}, tmuxColorOptions(tty)...)
	_, err = tea.NewProgram(model, options...).Run()
	if reviewDemo {
		_, _ = io.WriteString(tty, tui.ProgramStatusClear())
	}
	return err
}

func init() {
	rootCmd.AddCommand(viewCmd)
	viewCmd.Flags().StringVar(&reviewScope, "scope", "", "scope: all, staged, unstaged, COMMIT, A..B, or A...B (COMMIT compares with its first parent)")
	viewCmd.Flags().BoolVar(&reviewStaged, "staged", false, "view only changes staged in the index")
	viewCmd.Flags().BoolVar(&reviewUnstaged, "unstaged", false, "view unstaged and untracked changes")
	addDiffFlag(viewCmd)
	addFocusFlag(viewCmd)
	addWatchFlag(viewCmd)
	addHeaderFlags(viewCmd)
	viewCmd.Flags().StringArrayVar(&attachmentPaths, "attachment", nil, "Attach a document (repeatable)")
	reviewCmd.Flags().BoolVar(&reviewDemo, "demo", false, "try a review in the current terminal without saving")
	if err := reviewCmd.Flags().MarkHidden("demo"); err != nil {
		panic(err)
	}
}
