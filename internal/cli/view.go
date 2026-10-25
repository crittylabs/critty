package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/tui"
)

var reviewDemo bool

var viewCmd = &cobra.Command{
	Use:     "view [file[:LINE]]",
	Short:   "View changes or a document without reviewing or saving",
	Long:    "View Git changes, a document, a supplied diff, or attachments in the current terminal.  Search and display controls work, but review editing and external applications are disabled.  Nothing is saved.",
	Args:    cobra.MaximumNArgs(1),
	PreRunE: validateScopeFlags,
	RunE:    func(_ *cobra.Command, args []string) error { return runReviewInput(args, true) },
}

func transientReview(cmd *cobra.Command) bool { return cmd == viewCmd || reviewDemo }

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
	project := reviewProject(cfg.ProjectRoot)
	if headerFlagSpecified("project") {
		project = reviewProjectName
	}
	appCfg := tui.AppConfig{
		Context:   ctx,
		Watch:     reviewWatch,
		Documents: mode.documents,
		Project:   project,
		Topic:     reviewTopic,
		Version:   versionString(),
		ReadOnly:  !reviewDemo,
		Answers:   answersMode,
		Author:    cfg.Author,
		Split:     cfg.Split,
		Staged:    mode.staged,
		Source:    mode.source,
		Patch:     mode.patch,
		FocusPath: mode.focusPath,
		FocusLine: mode.focusLine,
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
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("this mode requires a foreground terminal: %w", err)
	}
	defer tty.Close()
	_, err = tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty)).Run()
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
