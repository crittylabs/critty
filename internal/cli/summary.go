package cli

import (
	"fmt"

	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
)

type summaryAttachment struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Revision string `json:"revision"`
	Removed  bool   `json:"removed,omitempty"`
	Image    string `json:"image,omitempty"`
}

type launcherFailure struct {
	Launcher string `json:"launcher"`
	Reason   string `json:"reason"`
}

type launchSummary struct {
	Launcher      string            `json:"launcher"`
	Failures      []launcherFailure `json:"failures"`
	TerminalError string            `json:"terminal_error,omitempty"`
}

// finishSummary indexes complete finish output without copying attachment bodies.
type finishSummary struct {
	Version       int                    `json:"version"`
	Session       string                 `json:"session"`
	Round         int                    `json:"round"`
	CWD           string                 `json:"cwd"`
	Workdir       string                 `json:"workdir,omitempty"`
	ImageBase     string                 `json:"image_base"`
	Verdict       review.Verdict         `json:"verdict"`
	Approved      bool                   `json:"approved"`
	ReviewComment *review.Comment        `json:"review_comment,omitempty"`
	Comments      []review.ListedComment `json:"comments"`
	Attachments   []summaryAttachment    `json:"attachments"`
	NextCommand   string                 `json:"next_command,omitempty"`
	Launch        *launchSummary         `json:"launch,omitempty"`
}

func validateSummaryPath(mode *reviewMode, sess *review.Session) error {
	if summaryPath == "" {
		return nil
	}
	var paths []string
	for _, d := range mode.documents {
		paths = append(paths, d.Source)
	}
	if err := validateExportPath(summaryPath, "--summary", mode, sess, paths); err != nil {
		return err
	}
	if resultPath != "" {
		summary, err := exportPath(summaryPath)
		if err != nil {
			return err
		}
		result, err := exportPath(resultPath)
		if err != nil {
			return err
		}
		if summary == result {
			return fmt.Errorf("--summary and --result must use different files")
		}
	}
	return nil
}

func writeFinishSummary(payload *ipc.FinishPayload, sess *review.Session, launch *launchSummary) error {
	if payload.Round < 1 || !payload.Verdict.Valid() || payload.Approved != (payload.Verdict == review.VerdictApprove) {
		return fmt.Errorf("invalid finish verdict or round")
	}
	documents, err := sess.LoadDocuments()
	if err != nil {
		return err
	}
	attachments := make([]summaryAttachment, 0, len(documents.Documents))
	for _, d := range documents.Documents {
		attachments = append(attachments, summaryAttachment{Path: d.Path(), Source: d.Source, Revision: d.Revision, Removed: d.Removed, Image: d.Image})
	}
	comments := payload.Comments
	if comments == nil {
		comments = []review.ListedComment{}
	}
	return review.WriteJSON(summaryPath, finishSummary{
		Version: 1, Session: sess.Key, Round: payload.Round,
		CWD: sess.Meta.CWD, Workdir: sess.Meta.Workdir, ImageBase: sess.Dir,
		Verdict: payload.Verdict, Approved: payload.Approved,
		ReviewComment: payload.ReviewComment, Comments: comments,
		Attachments: attachments, NextCommand: payload.NextCommand,
		Launch: launch,
	})
}
