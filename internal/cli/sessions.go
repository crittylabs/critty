package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

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
	var directory string
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List saved review sessions and their states",
		Long: `List saved reviews for the current project, including its linked worktrees.
Git projects share a common Git directory; non-Git projects use their directory.
Use --directory DIR to select the project there, --all to include every project,
or --json for structured output.  Sessions are grouped by project, with running
reviews first in each group.  Resume with critty sessions resume <id>; Critty
automatically uses the session's saved working directory.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("directory") && directory == "" {
				return fmt.Errorf("--directory requires a directory")
			}
			items, err := collectProjectSessions(directory, all)
			if err != nil {
				return err
			}
			slices.SortStableFunc(items, func(a, b sessionStatus) int {
				if a.ProjectID != b.ProjectID {
					if order := strings.Compare(a.ProjectName, b.ProjectName); order != 0 {
						return order
					}
					return strings.Compare(a.ProjectID, b.ProjectID)
				}
				if a.Running != b.Running {
					if a.Running {
						return -1
					}
					return 1
				}
				return strings.Compare(b.StartedAt, a.StartedAt)
			})
			if jsonOutput {
				return printJSON(cmd.OutOrStdout(), items)
			}
			if len(items) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No saved review sessions.")
				return err
			}
			rows := [][]string{{"PROJECT", "ID", "STATUS", "MODE", "ROUND", "TARGET"}}
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
				rows = append(rows, []string{clean(item.ProjectName), clean(item.Key), item.Status, clean(item.Mode), fmt.Sprint(item.Round), clean(target)})
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
