package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/review"
)

type sessionStatus struct {
	savedSession
	ProjectID   string         `json:"project_id"`
	ProjectName string         `json:"project_name"`
	Status      string         `json:"status"`
	Round       int            `json:"round,omitempty"`
	Verdict     review.Verdict `json:"verdict,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func newSessionsCommand() *cobra.Command {
	var all, jsonOutput bool
	var directory, sortOrder string
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List saved review sessions and their states",
		Long: `List saved reviews for the current project, including its linked worktrees.
Git projects share a common Git directory; non-Git projects use their directory.
Use --directory DIR to select the project there, --all to include every project,
or --json for structured output.  Sessions are grouped by project, with running
reviews first in each group, then newest first.  Use --sort time for newest-first
order across projects, without prioritizing running reviews.  STARTED shows the
session's start time in your local timezone, including its UTC offset.
Resume with critty sessions resume <id>.  Within the same Git project, Critty
uses your current checkout, even if the original worktree has been removed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sortOrder != "project" && sortOrder != "time" {
				return fmt.Errorf("invalid sort order %q: use project or time", sortOrder)
			}
			if cmd.Flags().Changed("directory") && directory == "" {
				return fmt.Errorf("--directory requires a directory")
			}
			items, err := collectProjectSessions(directory, all)
			if err != nil {
				return err
			}
			sortSessionStatuses(items, sortOrder)
			if jsonOutput {
				return printJSON(cmd.OutOrStdout(), items)
			}
			if len(items) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No saved review sessions.")
				return err
			}
			rows := [][]string{{"PROJECT", "ID", "STATUS", "STARTED", "MODE", "ROUND", "TARGET"}}
			clean := func(s string) string { return strings.Join(strings.Fields(ansi.Strip(s)), " ") }
			for _, item := range items {
				target := item.Branch
				if item.Mode == "files" && len(item.Args) > 0 {
					target = item.Args[0]
				} else if item.PlanSlug != "" {
					target = item.PlanSlug
				}
				if item.Topic != nil && *item.Topic != "" {
					target = *item.Topic
				}
				started := "-"
				if stamp := sessionStartTime(item.StartedAt); !stamp.IsZero() {
					started = stamp.Local().Format(time.RFC3339)
				}
				rows = append(rows, []string{clean(item.ProjectName), clean(item.Key), item.Status, started, clean(item.Mode), fmt.Sprint(item.Round), clean(target)})
			}
			widths := make([]int, len(rows[0]))
			for _, row := range rows {
				for i, cell := range row {
					widths[i] = max(widths[i], ansi.StringWidth(cell))
				}
			}
			for i, row := range rows {
				var line strings.Builder
				for j, cell := range row {
					line.WriteString(cell)
					if j < len(row)-1 {
						line.WriteString(strings.Repeat(" ", widths[j]-ansi.StringWidth(cell)+2))
					}
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line.String()); err != nil {
					return err
				}
				if i > 0 && (items[i-1].Error != "" || items[i-1].HistoryError != "") {
					item := items[i-1]
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  "+clean(item.Error+" "+item.HistoryError)); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	list := &cobra.Command{Use: "list", Short: cmd.Short, Long: cmd.Long, Args: cobra.NoArgs, RunE: cmd.RunE}
	for _, command := range []*cobra.Command{cmd, list} {
		command.Flags().BoolVar(&all, "all", false, "include reviews from all projects")
		command.Flags().StringVar(&directory, "directory", "", "list reviews for the project containing DIR (default current project)")
		command.Flags().BoolVar(&jsonOutput, "json", false, "print session states as JSON")
		command.Flags().StringVar(&sortOrder, "sort", "project", "sort by project (running first) or time (newest start first)")
		command.MarkFlagsMutuallyExclusive("all", "directory")
	}
	cmd.AddCommand(list, &cobra.Command{
		Use: "stop <id>", Short: "Stop a review TUI and keep its saved state", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return stopReview(args[0]) },
	}, &cobra.Command{
		Use: "remove <id>", Short: "Delete a stopped saved review and its attachments", Args: cobra.ExactArgs(1),
		Long: "Delete a stopped saved review, including its comments, replies, attachments, and form answers.\nTask members must be managed through critty history instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := review.ResolveTarget("", args[0])
			if err != nil {
				return err
			}
			if err := clearSavedReview(sess); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed review %s\n", sess.Key)
			return err
		},
	})
	return cmd
}

func sortSessionStatuses(items []sessionStatus, order string) {
	slices.SortStableFunc(items, func(a, b sessionStatus) int {
		if order == "project" {
			if a.ProjectID != b.ProjectID {
				if cmp := strings.Compare(a.ProjectName, b.ProjectName); cmp != 0 {
					return cmp
				}
				return strings.Compare(a.ProjectID, b.ProjectID)
			}
			if a.Running != b.Running {
				if a.Running {
					return -1
				}
				return 1
			}
		}
		if cmp := sessionStartTime(b.StartedAt).Compare(sessionStartTime(a.StartedAt)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Key, b.Key)
	})
}

func sessionStartTime(value string) time.Time {
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return stamp
}

func inspectSessionStatus(entry savedSession) sessionStatus {
	item := sessionStatus{savedSession: entry, Status: "stopped"}
	sess, err := review.OpenSessionFromEntry(entry.SessionEntry)
	if err == nil {
		_, err = os.Stat(sess.Path())
	}
	if err != nil {
		item.Status, item.Error = "unavailable", err.Error()
	} else {
		item.Round = sess.CJ.ReviewRound
		if sess.CJ.RoundState.Finished {
			item.Verdict = sess.CJ.RoundState.Verdict
			switch item.Verdict {
			case review.VerdictApprove:
				item.Status = "approved"
			case review.VerdictReject:
				item.Status = "changes-requested"
			case review.VerdictComment:
				item.Status = "feedback"
			default:
				item.Status = "submitted"
			}
		}
	}
	if item.Running {
		item.Status = "running"
	}
	return item
}

func init() {
	cmd := newSessionsCommand()
	cmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(cmd)
}
