package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/review"
)

var reviewTask string

func historyOutput(cmd *cobra.Command, result any, operationErr error) error {
	if operationErr != nil {
		// Partial organization is durable and must remain visible on failure.
		return errors.Join(operationErr, printJSON(cmd.OutOrStdout(), map[string]any{"result": result, "error": operationErr.Error()}))
	}
	return printJSON(cmd.OutOrStdout(), result)
}

func taskReviewConfig(cfg *config.Config, sess *review.Session) (*config.Config, error) {
	task, err := history.New(sess.Meta.CWD).ForSession(sess.Key)
	if err != nil {
		if errors.Is(err, history.ErrHistoryUnavailable) {
			fmt.Fprintf(os.Stderr, "critty: history unavailable; retaining review %s: %v\n", sess.Key, err)
			copy := *cfg
			copy.CleanupOnApprove = false
			return &copy, nil
		}
		return nil, err
	}
	if task == nil {
		return cfg, nil
	}
	if task.Pruning {
		return nil, fmt.Errorf("task %s has interrupted organization; run critty history finish %s --action %s", task.ID, task.ID, task.Action)
	}
	if task.CompletedAt != "" {
		return nil, fmt.Errorf("task %s is complete; run critty history reopen %s before continuing", task.ID, task.ID)
	}
	copy := *cfg
	copy.CleanupOnApprove = false
	return &copy, nil
}

func historyContext() (*history.Store, history.Policy, error) {
	cfg, err := config.LoadCurrent()
	if err != nil {
		return nil, history.Policy{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, history.Policy{}, err
	}
	store := history.New(cwd)
	store.Project = filepath.Base(cfg.ProjectRoot)
	return store, cfg.History, nil
}

func newHistoryCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "history", Short: "Group, save, and organize reviews by task",
		Long: "Group several review sessions into one task.  Retention runs only at explicit task boundaries.\n\n" +
			"Actions are auto, keep, ask, archive (save then remove originals), and delete.  Auto is the default.  " +
			"Ask returns JSON with decision_required=true and makes no destructive change; choose an action once for the task and rerun finish or clean.\n\n" +
			"Auto retains backups quietly, asks about completed tasks beyond keep_days, and rotates the oldest completed backups at capacity to 80% of max_bytes.  Archives and ongoing or busy reviews are protected.\n\n" +
			"Usage counts allocated disk blocks for review files and directories, counting hard links once.\n\n" +
			"Global history settings: start, end, keep_days (default 30), max_bytes (default 1 GiB), and archive_dir.  " +
			"Project files cannot change retention policy.  Start/clean consider only completed tasks in this directory, based on their most recent saved review update.  " +
			"Fixed finish actions apply to the named task; auto also checks completed backups in this directory.  Active or busy reviews cannot be exported or removed.\n\n" +
			"Archives default to the Critty state directory's archives folder, with a dated project-and-task folder containing review.md, attachments/, assets/, sessions/, and manifest.json.  Each document has its own assets/ for referenced images.  " +
			"Export keeps originals; the archive action verifies saved files before removing them.  Archives preserve saved state, not every historical round or original host completion evidence.",
	}
	var startAction string
	var project string
	start := &cobra.Command{Use: "start <name>", Short: "Start a task and apply its start retention policy", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("action") {
				policy.Start = startAction
			}
			if cmd.Flags().Changed("project") {
				store.Project = project
			}
			result, err := store.Start(args[0], policy)
			return historyOutput(cmd, result, err)
		},
	}
	start.Flags().StringVar(&startAction, "action", "", "override start policy: auto, keep, ask, archive, delete")
	start.Flags().StringVar(&project, "project", "", "project name for this task (default: repository or directory name)")
	var finishAction string
	finish := &cobra.Command{Use: "finish <task-id>", Short: "Complete a task and keep, save, or delete its reviews", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			action := policy.End
			if cmd.Flags().Changed("action") {
				action = finishAction
			}
			policy.End = action
			result, err := store.FinishPolicy(args[0], policy)
			return historyOutput(cmd, result, err)
		},
	}
	finish.Flags().StringVar(&finishAction, "action", "", "override end policy: auto, keep, ask, archive, delete")
	var cleanAction string
	var days int
	clean := &cobra.Command{Use: "clean", Short: "Apply retention to old completed tasks in this directory", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			action := policy.Start
			if cmd.Flags().Changed("action") {
				action = cleanAction
			}
			age := policy.KeepDays
			if cmd.Flags().Changed("days") {
				age = days
			}
			if action == "auto" {
				policy.Start, policy.KeepDays = action, age
				result, err := store.Maintain(policy)
				return historyOutput(cmd, result, err)
			}
			result, err := store.Clean(action, age, policy.ArchiveDir)
			return historyOutput(cmd, result, err)
		},
	}
	clean.Flags().StringVar(&cleanAction, "action", "", "override start policy: auto, keep, ask, archive, delete")
	clean.Flags().IntVar(&days, "days", 30, "minimum idle age in days; 0 includes all completed tasks")
	var sessionID string
	add := &cobra.Command{Use: "add <task-id>", Short: "Associate an existing review with a task", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			if sessionID == "" {
				return fmt.Errorf("--session is required")
			}
			if err := store.Add(args[0], sessionID); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), map[string]string{"task": args[0], "session": sessionID})
		},
	}
	add.Flags().StringVar(&sessionID, "session", "", "saved review session ID")
	var output string
	var allowIncomplete bool
	export := &cobra.Command{Use: "export <task-id>", Short: "Save a portable Markdown archive while retaining original reviews", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			dir := policy.ArchiveDir
			if cmd.Flags().Changed("output") {
				dir = output
			}
			path, err := store.Export(args[0], dir, allowIncomplete)
			return historyOutput(cmd, map[string]string{"archive": path}, err)
		},
	}
	export.Flags().StringVar(&output, "output", "", "parent directory for the dated archive folder")
	export.Flags().BoolVar(&allowIncomplete, "allow-incomplete", false, "record missing reviews or external images as warnings; retain originals")
	var detachSession string
	detach := &cobra.Command{Use: "detach <task-id>", Short: "Remove a review from a task without deleting its data", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			if detachSession == "" {
				return fmt.Errorf("--session is required")
			}
			if err := store.Detach(args[0], detachSession); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), map[string]string{"task": args[0], "detached": detachSession})
		},
	}
	detach.Flags().StringVar(&detachSession, "session", "", "review to detach, including an already removed review")
	list := &cobra.Command{Use: "list", Short: "List saved tasks in this directory", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			tasks, err := store.List()
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), tasks)
		},
	}
	reopen := &cobra.Command{Use: "reopen <task-id>", Short: "Continue a completed task whose original reviews were retained", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := historyContext()
			if err != nil {
				return err
			}
			task, err := store.Reopen(args[0])
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), task)
		},
	}
	usage := &cobra.Command{Use: "usage", Short: "Estimate allocated review disk usage without changing records", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, policy, err := historyContext()
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), store.Usage(policy.MaxBytes))
		},
	}
	command.AddCommand(start, add, detach, finish, clean, export, list, reopen, usage)
	return command
}

func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Lookup("task") == nil || reviewTask == "" {
			return nil
		}
		store, _, err := historyContext()
		if err != nil {
			return err
		}
		return store.Validate(reviewTask)
	}
	rootCmd.AddCommand(newHistoryCommand())
	for _, command := range []*cobra.Command{rootCmd, reviewCmd, planCmd} {
		command.Flags().StringVar(&reviewTask, "task", "", "associate this review with a history task ID")
	}
}
