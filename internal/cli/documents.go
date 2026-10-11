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

func addAttachmentFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&resultPath, "result", "", "Save the self-contained form result as JSON")
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
	path, err := filepath.Abs(resultPath)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	rel, err := filepath.Rel(sess.Dir, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("--result must be outside the review session directory")
	}
	paths = append(paths, mode.docPath)
	for _, file := range mode.files {
		paths = append(paths, file.Path)
	}
	for _, target := range paths {
		if target == "" {
			continue
		}
		abs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if abs == path {
			return fmt.Errorf("--result would overwrite reviewed input: %s", target)
		}
	}
	return nil
}
