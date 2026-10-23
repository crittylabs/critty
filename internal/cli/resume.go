package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/review"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume a saved review session",
	Long: `Resume a saved Git, document, diff, or plan review by ID.
Critty automatically uses its original working directory and restores input,
comments, replies, attachments, and form answers.  Relative attachment, diff,
and output paths are resolved from the directory where you run this command.

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
	return withSessionDirectory(key, args, func(args []string) error {
		if reviewDiff != "" || reviewNull {
			return resumeDiff(key, args)
		}
		return reconnectSession(key)
	})
}

func resumeEntry(key string) (*review.SessionEntry, error) {
	if !review.ValidSessionKey(key) {
		return nil, fmt.Errorf("invalid session ID %q", key)
	}
	return review.ReadSessionEntry(key)
}

func withSessionDirectory(key string, args []string, run func([]string) error) (err error) {
	entry, err := resumeEntry(key)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if cwd == entry.CWD {
		return run(args)
	}
	oldDiff, oldResult, oldSummary, oldAttachments := reviewDiff, resultPath, summaryPath, attachmentPaths
	oldLaunchers := reviewLauncherNames
	defer func() {
		reviewDiff, resultPath, summaryPath, attachmentPaths = oldDiff, oldResult, oldSummary, oldAttachments
		reviewLauncherNames = oldLaunchers
	}()
	paths := []*string{&resultPath, &summaryPath}
	if reviewDiff != "-" {
		paths = append(paths, &reviewDiff)
	}
	for _, path := range paths {
		if *path != "" {
			if *path, err = filepath.Abs(*path); err != nil {
				return err
			}
		}
	}
	attachmentPaths = slices.Clone(attachmentPaths)
	for i, path := range attachmentPaths {
		if attachmentPaths[i], err = filepath.Abs(path); err != nil {
			return err
		}
	}
	args = slices.Clone(args)
	for i, path := range args {
		if args[i], err = filepath.Abs(path); err != nil {
			return err
		}
	}
	reviewLauncherNames = slices.Clone(reviewLauncherNames)
	for i, name := range reviewLauncherNames {
		if path, ok := strings.CutPrefix(name, "custom:"); ok && strings.Contains(path, "/") {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			reviewLauncherNames[i] = "custom:" + absolute
		}
	}
	if err := os.Chdir(entry.CWD); err != nil {
		return fmt.Errorf("opening saved review directory: %w", err)
	}
	defer func() {
		if restoreErr := os.Chdir(cwd); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restoring working directory: %w", restoreErr))
		}
	}()
	return run(args)
}

func resumingSessionID(cmd *cobra.Command, args []string) string {
	switch cmd {
	case resumeCmd:
		if len(args) > 0 {
			return args[0]
		}
	case rootCmd:
		return rootSession
	case planCmd:
		return planSession
	}
	return ""
}

func init() {
	addDiffFlag(resumeCmd)
	addFocusFlag(resumeCmd)
	addWatchFlag(resumeCmd)
	addHeaderFlags(resumeCmd)
	addAttachmentFlags(resumeCmd)
}
