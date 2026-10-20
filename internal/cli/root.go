package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/migration"
	"github.com/crittylabs/critty/internal/review"
)

var rootSession string

var rootCmd = &cobra.Command{
	Use:     "critty [file[:LINE]]",
	Version: versionString(),
	Short:   "Review code, make choices, and approve actions with your coding agent",
	Long: "Critty is a terminal UI for reviewing code and documents, making choices, " +
		"and approving actions with your coding agent. " +
		"Leave inline feedback, choose between alternatives, and authorize specific actions, " +
		"with or without a code diff. " +
		"It provides an interactive TUI for humans and scriptable CLI commands for agents.\n\n" +
		"Run `critty` to review the current git changes, `critty --staged` to review only the index, " +
		"`critty <file>` to review a document, " +
		"or `critty --session <id>` to resume a saved review.  Use `critty view` for a read-only view in the current terminal without saving a session.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootSession != "" {
			if len(args) > 0 && reviewDiff == "" && !reviewNull {
				return fmt.Errorf("--session cannot be combined with a file argument")
			}
			if reviewStaged || reviewUnstaged || reviewScope != "" {
				return fmt.Errorf("--session cannot be combined with --staged, --unstaged, or --scope")
			}
			if reviewDiff != "" || reviewNull {
				return resumeDiff(rootSession, args)
			}
			return reconnectSession(rootSession)
		}
		return runReview(args)
	},
}

// reconnectSession restores a saved review in its original working directory.
func reconnectSession(key string) error {
	cfg, err := config.LoadCurrent()
	if err != nil {
		return err
	}
	sess, mode, err := loadReviewSession(key)
	if err != nil {
		return err
	}
	if ipc.Alive(review.SocketPathFor(key)) {
		return fmt.Errorf("review %s is already active", key)
	}
	return runReviewFlow(cfg, sess, mode)
}

func Execute() int {
	preRun := rootCmd.PersistentPreRunE
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if transientReview(cmd) {
			if err := validateTransientFlags(cmd); err != nil {
				return err
			}
			if err := os.Setenv("GIT_OPTIONAL_LOCKS", "0"); err != nil {
				return err
			}
		} else if err := runStartupMigration(); err != nil {
			return err
		}
		if preRun != nil {
			return preRun(cmd, args)
		}
		return nil
	}
	defer func() { rootCmd.PersistentPreRunE = preRun }()
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func runStartupMigration() error {
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		project = strings.TrimSpace(string(out))
	}
	if err := migration.Run(project, os.Stderr); err != nil {
		return err
	}
	return nil
}

func init() {
	rootCmd.PreRunE = validateScopeFlags
	rootCmd.Flags().StringVar(&reviewScope, "scope", "", "review scope: all (default), staged, unstaged, A..B, or A...B (omitted B means HEAD)")
	rootCmd.Flags().StringVar(&rootSession, "session", "", "resume a saved review session by ID")
	rootCmd.Flags().BoolVar(&reviewStaged, "staged", false, "review only changes staged in the index (alias for --scope=staged)")
	rootCmd.Flags().BoolVar(&reviewUnstaged, "unstaged", false, "review unstaged and untracked changes (alias for --scope=unstaged)")
	addDiffFlag(rootCmd)
	addFocusFlag(rootCmd)
	addHeaderFlags(rootCmd)
	addAttachmentFlags(rootCmd)
}
