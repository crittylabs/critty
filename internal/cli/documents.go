package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/review"
)

var attachmentPaths []string
var clearAttachments bool

func addAttachmentFlags(cmd *cobra.Command) {
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
