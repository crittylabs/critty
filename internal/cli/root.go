package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/ipc"
	"github.com/knu/tcrit/internal/review"
)

var rootSession string

var rootCmd = &cobra.Command{
	Use:     "tcrit [file[:LINE]]",
	Version: versionString(),
	Short:   "Review code changes and documents from the terminal",
	Long: "TCrit is a terminal-based review tool for code changes and documents. " +
		"It provides an interactive TUI for humans and scriptable CLI commands for agents.\n\n" +
		"Run `tcrit` to review the current git changes, `tcrit --staged` to review only the index, " +
		"`tcrit <file>` to review a document, " +
		"or `tcrit --session <id>` to resume a saved review.",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootSession != "" {
			if len(args) > 0 && reviewDiff == "" {
				return fmt.Errorf("--session cannot be combined with a file argument")
			}
			if reviewStaged || reviewUnstaged || reviewScope != "" {
				return fmt.Errorf("--session cannot be combined with --staged, --unstaged, or --scope")
			}
			if reviewDiff != "" {
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
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
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
