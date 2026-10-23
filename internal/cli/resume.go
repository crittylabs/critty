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
Within the same Git project, Critty uses your current working directory,
even when the original worktree has been removed.  Otherwise it restores the
last workspace.  Comments, replies, attachments, and form answers are retained.
Relative attachment, diff,
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
	directory := entry.WorkingDirectory()
	if sameSessionDirectory(cwd, directory) || sameSessionProject(*entry, cwd) {
		return run(args)
	}
	if _, err := os.Stat(directory); os.IsNotExist(err) && entry.Workdir != "" {
		directory = entry.CWD
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
	if err := os.Chdir(directory); err != nil {
		return fmt.Errorf("opening saved review directory: %w; resume from another checkout of the same Git project", err)
	}
	defer func() {
		if restoreErr := os.Chdir(cwd); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restoring working directory: %w", restoreErr))
		}
	}()
	return run(args)
}

func sameSessionDirectory(a, b string) bool {
	if a == b {
		return true
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

func sameSessionProject(entry review.SessionEntry, directory string) bool {
	common := entry.GitCommonDir
	if common == "" {
		common = reviewGitCommonDir(entry.WorkingDirectory())
	}
	return common != "" && common == reviewGitCommonDir(directory)
}

// Only paths inside the previous workspace move.  External inputs stay external.
func rebaseSessionPath(path, from, to string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(canonicalProjectDirectory(from), canonicalProjectDirectory(path))
	if err != nil || !filepath.IsLocal(rel) {
		return path
	}
	return filepath.Join(to, rel)
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
