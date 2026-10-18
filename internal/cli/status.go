package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

var statusCode bool

// docStatus is the JSON shape for a single-document review status.
type docStatus struct {
	File     string           `json:"file"`
	Comments []review.Comment `json:"comments"`
}

var statusCmd = &cobra.Command{
	Use:   "status [file]",
	Short: "Show review status as JSON",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}

		if statusCode {
			return runCodeStatus(cfg)
		}

		if len(args) == 0 {
			return listReviewSessions()
		}

		filePath := args[0]

		sess, err := review.ResolveDocument(cfg.Output, filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		comments := sess.FileComments(filePath)
		if comments == nil {
			comments = []review.Comment{}
		}
		return printJSON(os.Stdout, docStatus{File: filePath, Comments: comments})
	},
}

func runCodeStatus(cfg *config.Config) error {
	status, err := review.AggregateStatus(cfg.Output)
	if err != nil {
		return err
	}
	return printJSON(os.Stdout, status)
}

func printJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusCode, "code", false, "show aggregate status for the code review session")
}

func listReviewSessions() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	entries, err := review.ListSessionEntries()
	if err != nil {
		return err
	}
	type savedSession struct {
		review.SessionEntry
		Running      bool   `json:"running"`
		Task         string `json:"task,omitempty"`
		Ungrouped    bool   `json:"ungrouped"`
		HistoryError string `json:"history_error,omitempty"`
	}
	result := []savedSession{}
	for _, entry := range entries {
		if entry.CWD == cwd {
			item := savedSession{SessionEntry: entry, Running: ipc.Alive(review.SocketPathFor(entry.Key))}
			task, err := history.New(cwd).ForSession(entry.Key)
			if err != nil {
				item.HistoryError = err.Error()
			} else if task != nil {
				item.Task = task.ID
			} else {
				item.Ungrouped = true
			}
			result = append(result, item)
		}
	}
	return printJSON(os.Stdout, result)
}
