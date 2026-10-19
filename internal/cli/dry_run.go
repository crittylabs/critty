package cli

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/tui"
)

var reviewDryRun bool

func validateDryRunFlags(cmd *cobra.Command) error {
	for _, name := range []string{"session", "task", "result", "summary", "terminal-request", "launcher", "clear-attachments"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--dry-run cannot be combined with --%s", name)
		}
	}
	return nil
}

// runDryReview opens the current terminal without session storage or an IPC server.
func runDryReview(cfg *config.Config, mode *reviewMode) error {
	if err := resolveFocus(mode); err != nil {
		return err
	}
	project := reviewProject(cfg.ProjectRoot)
	if headerFlagSpecified("project") {
		project = reviewProjectName
	}
	appCfg := tui.AppConfig{
		Documents: mode.documents,
		Project:   project,
		Topic:     reviewTopic,
		Version:   versionString(),
		ReadOnly:  true,
		Split:     cfg.Split,
		Staged:    mode.staged,
		Source:    mode.source,
		Patch:     mode.patch,
		FocusPath: mode.focusPath,
		FocusLine: mode.focusLine,
	}
	var model tui.AppModel
	if mode.code() {
		model = tui.NewCodeReviewApp(mode.files, mode.ref, appCfg)
	} else {
		model = tui.NewApp(mode.docPath, appCfg)
	}
	// Stdin may contain a patch.  Always read interaction from the foreground TTY.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("--dry-run requires a foreground terminal: %w", err)
	}
	defer tty.Close()
	_, err = tea.NewProgram(model, tea.WithInput(tty), tea.WithOutput(tty)).Run()
	return err
}

func init() {
	for _, cmd := range []*cobra.Command{rootCmd, reviewCmd} {
		cmd.Flags().BoolVar(&reviewDryRun, "dry-run", false, "view in the current terminal without saving a session or enabling edits")
	}
}
