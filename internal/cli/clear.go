package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

var clearCode bool
var clearAll bool
var clearSession string

var clearCmd = &cobra.Command{
	Use:   "clear [file]",
	Short: "Clear all comments for a document or code review session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}

		if clearAll {
			if clearCode || len(args) > 0 || clearSession != "" {
				return fmt.Errorf("--all cannot be combined with --code or a file argument")
			}
			return runClearAll()
		}
		if clearSession != "" {
			if clearCode || len(args) > 0 {
				return fmt.Errorf("--session cannot be combined with --code or a file")
			}
			sess, err := review.ResolveTarget(cfg.Output, clearSession)
			if err != nil {
				return err
			}
			return clearSavedReview(sess)
		}

		if clearCode {
			return runCodeClear(cfg)
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for code review session, --all to reset all review state)")
		}

		filePath := args[0]

		sess, err := review.ResolveDocument(cfg.Output, filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		count := len(sess.FileComments(filePath))
		if err := clearSavedReview(sess); err != nil {
			return fmt.Errorf("clearing review: %w", err)
		}

		fmt.Printf("Cleared %d comment(s) for %s\n", count, filePath)
		return nil
	},
}

func runCodeClear(cfg *config.Config) error {
	sess, err := review.ResolveCode(cfg.Output)
	if err != nil {
		return err
	}

	total := sess.CJ.TotalComments()
	fileCount := len(sess.CJ.Files)
	if err := clearSavedReview(sess); err != nil {
		return fmt.Errorf("clearing review: %w", err)
	}

	fmt.Printf("Cleared %d comment(s) across %d file(s)\n", total, fileCount)
	return nil
}

// runClearAll deletes all review sessions registered for the current working
// directory.  Use it when starting a fresh review task.
func runClearAll() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving cwd: %w", err)
	}

	entries, err := review.ListSessionEntries()
	if err != nil {
		return err
	}

	sessions := []*review.Session{}
	for _, e := range entries {
		if e.CWD == cwd && ipc.Alive(review.SocketPathFor(e.Key)) {
			return fmt.Errorf("review %s is active; stop it before clearing", e.Key)
		}
	}
	for _, e := range entries {
		if e.CWD != cwd {
			continue
		}
		dir := review.Dir("", e.Key)
		if e.ReviewPath != "" {
			dir = filepath.Dir(e.ReviewPath)
		}
		sessions = append(sessions, &review.Session{Key: e.Key, Dir: dir})
	}
	if len(sessions) > 0 {
		if err := history.New(cwd).ClearSessions(sessions); err != nil {
			return err
		}
	}

	fmt.Printf("Removed %d review session(s)\n", len(sessions))
	return nil
}

func init() {
	rootCmd.AddCommand(clearCmd)
	clearCmd.Flags().BoolVar(&clearCode, "code", false, "clear the code review session")
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "delete all review sessions for the current directory")
	clearCmd.Flags().StringVar(&clearSession, "session", "", "delete one saved review")
}

func clearSavedReview(sess *review.Session) error {
	if ipc.Alive(review.SocketPathFor(sess.Key)) {
		return fmt.Errorf("review %s is active; stop it before clearing", sess.Key)
	}
	return history.ClearSession(sess)
}
