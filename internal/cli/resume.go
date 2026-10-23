package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume a saved review session",
	Long: `Resume a saved Git, document, diff, or plan review from its original directory.
The session restores its input, comments, replies, attachments, and form answers.

Use --attachment repeatedly to replace the complete attachment list, or
--clear-attachments to remove it.  For supplied diffs, --diff[=FILE] replaces
the saved input from a file or stdin (-).  To add a new version of a plan,
use critty plan --session <id> <file> instead.

The legacy form critty --session <id> remains supported.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return resumeReview(args[0], args[1:])
	},
}

func resumeReview(key string, args []string) error {
	if len(args) > 0 && reviewDiff == "" && !reviewNull {
		return fmt.Errorf("a resumed review cannot take a file argument; use --diff to replace a supplied diff or plan --session to update a plan")
	}
	if reviewDiff != "" || reviewNull {
		return resumeDiff(key, args)
	}
	return reconnectSession(key)
}

func init() {
	addDiffFlag(resumeCmd)
	addFocusFlag(resumeCmd)
	addWatchFlag(resumeCmd)
	addHeaderFlags(resumeCmd)
	addAttachmentFlags(resumeCmd)
}
