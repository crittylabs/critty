package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crittylabs/critty/internal/review"
)

var attachmentPaths []string
var clearAttachments bool
var resultPath string
var summaryPath string

func addAttachmentFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&resultPath, "result", "", "Save the self-contained form result as JSON")
	cmd.Flags().StringVar(&summaryPath, "summary", "", "Save verdict, all threads, and attachment identities as JSON")
	cmd.Flags().StringArrayVar(&attachmentPaths, "attachment", nil, "Attach a document before the review (repeatable)")
	cmd.Flags().BoolVar(&clearAttachments, "clear-attachments", false, "remove all attached documents from a saved review")
}

func prepareDocuments(mode *reviewMode) error {
	if mode.documentsPrepared {
		return nil
	}
	if len(attachmentPaths) > 0 && clearAttachments {
		return fmt.Errorf("--attachment cannot be combined with --clear-attachments")
	}
	if clearAttachments && mode.sessionKey == "" {
		return fmt.Errorf("--clear-attachments requires --session")
	}
	if len(attachmentPaths) == 0 && !clearAttachments {
		mode.documentsPrepared = true
		return nil
	}
	docs, err := review.ReadAttachedDocuments(attachmentPaths)
	if err != nil {
		return err
	}
	mode.documents, mode.replaceDocuments = docs, true
	mode.documentsPrepared = true
	return nil
}

func validateResultPath(mode *reviewMode, sess *review.Session) error {
	if resultPath == "" {
		return nil
	}
	hasForms := false
	var paths []string
	for _, d := range mode.documents {
		items, err := review.ParseDocumentForm(d)
		if err != nil {
			return err
		}
		hasForms = hasForms || len(items) > 0
		paths = append(paths, d.Source)
	}
	if !hasForms {
		return fmt.Errorf("--result requires an interactive attachment form")
	}
	return validateExportPath(resultPath, "--result", mode, sess, paths)
}

func exportPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(parent, filepath.Base(path))
	}
	return path, nil
}

func validateExportPath(target, flag string, mode *reviewMode, sess *review.Session, paths []string) error {
	path, err := exportPath(target)
	if err != nil {
		return err
	}
	dir, err := exportPath(sess.Dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must be outside the review session directory", flag)
	}
	paths = append(paths, mode.docPath)
	for _, file := range mode.files {
		paths = append(paths, file.Path)
	}
	for _, target := range paths {
		if target == "" {
			continue
		}
		abs, err := exportPath(target)
		if err != nil {
			return err
		}
		if abs == path {
			return fmt.Errorf("%s would overwrite reviewed input: %s", flag, target)
		}
	}
	return nil
}
