package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/migration"
	"github.com/crittylabs/critty/internal/review"
)

var rootSession string
var maxWrapWidth int
var maxWrapWidthFlag *pflag.Flag

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
		"`critty <file>` to view a document without saving, `critty review <file>` to review it, " +
		"or `critty sessions resume <id>` to resume a saved review.  Use `critty view` for a read-only view in the current terminal without saving a session.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRootFiles,
	SilenceUsage:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootSession != "" {
			if reviewStaged || reviewUnstaged || reviewScope != "" {
				return fmt.Errorf("--session cannot be combined with --staged, --unstaged, or --scope")
			}
			return resumeReview(rootSession, args)
		}
		if defaultFileView(args) {
			return runReviewInput(args, true)
		}
		return runReview(args)
	},
}

func completeRootFiles(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// When no command matches, let the shell handle paths, quoting, and directories.
	commandNames := make(map[string]bool)
	matchingCommand := false
	for _, sub := range cmd.Commands() {
		commandNames[sub.Name()] = true
		for _, alias := range sub.Aliases {
			commandNames[alias] = true
		}
		if (sub.IsAvailableCommand() || sub.Name() == "help") && strings.HasPrefix(sub.Name(), toComplete) {
			matchingCommand = true
		}
	}
	if !matchingCommand {
		return nil, cobra.ShellCompDirectiveDefault
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, cobra.ShellCompDirectiveKeepOrder
	}
	var files []cobra.Completion
	directive := cobra.ShellCompDirectiveKeepOrder | cobra.ShellCompDirectiveNoFileComp
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.HasPrefix(name, toComplete) || strings.ContainsAny(name, "\t\r\n") {
			continue
		}
		directory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(name)
			directory = err == nil && info.IsDir()
		}
		if directory {
			name += "/"
			// Keep directory candidates open for the next path component.
			directive |= cobra.ShellCompDirectiveNoSpace
		} else if commandNames[name] {
			name = "./" + name
		}
		files = append(files, name)
	}
	// Cobra appends these after its command candidates; preserve that order.
	return files, directive
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
		if maxWrapWidth < 0 {
			return fmt.Errorf("--max-wrap-width must be nonnegative")
		}
		if transientReview(cmd, args) {
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
	rootCmd.PersistentFlags().IntVar(&maxWrapWidth, "max-wrap-width", 0, "maximum document text wrapping width in columns, excluding the gutter (0: unlimited)")
	maxWrapWidthFlag = rootCmd.PersistentFlags().Lookup("max-wrap-width")
	rootCmd.PreRunE = validateScopeFlags
	rootCmd.Flags().StringVar(&reviewScope, "scope", "", "review scope: all (default), staged, unstaged, COMMIT, A..B, or A...B (omitted B means HEAD; COMMIT compares with its first parent)")
	rootCmd.Flags().StringVar(&rootSession, "session", "", "resume a saved review session by ID")
	rootCmd.Flags().BoolVar(&reviewStaged, "staged", false, "review only changes staged in the index (alias for --scope=staged)")
	rootCmd.Flags().BoolVar(&reviewUnstaged, "unstaged", false, "review unstaged and untracked changes (alias for --scope=unstaged)")
	addDiffFlag(rootCmd)
	addFocusFlag(rootCmd)
	addWatchFlag(rootCmd)
	addHeaderFlags(rootCmd)
	addAttachmentFlags(rootCmd)
}
