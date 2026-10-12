package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/config"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/prompt"
	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/tui"
)

// runTUISession runs the review TUI in this process.  When serving, it
// listens on the session socket so blocking agent clients receive finish
// payloads and can start new rounds; otherwise it returns the last finish
// payload for the caller to print (inline mode).
func runTUISession(cfg *config.Config, sess *review.Session, mode *reviewMode, serving bool) (*ipc.FinishPayload, error) {
	documents, err := sess.LoadDocuments()
	if err != nil {
		return nil, err
	}
	finishCh := make(chan tui.FinishEvent, 4)
	appCfg := tui.AppConfig{
		Documents: documents.Documents,
		Project:   reviewProjectLabel(cfg.ProjectRoot, sess),
		Topic:     reviewTopicLabel(sess, mode),
		Version:   versionString(),
		Session:   sess,
		Author:    cfg.Author,
		Staged:    mode.staged,
		Source:    mode.source,
		FinishCh:  finishCh,
		Patch:     mode.patch,
		FocusPath: mode.focusPath,
		FocusLine: mode.focusLine,
	}

	var model tui.AppModel
	if mode.code() {
		model = tui.NewCodeReviewApp(mode.files, mode.ref, appCfg)
	} else {
		model = tui.NewApp(mode.docPath, appCfg)
	}
	var options []tea.ProgramOption
	if mode.patch != nil && !serving {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return nil, withCodexWrapperHint(fmt.Errorf("opening review terminal: %w", err))
		}
		defer func() {
			if err := tty.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "critty: closing review terminal: %v\n", err)
			}
		}()
		options = append(options, tea.WithInput(tty), tea.WithOutput(tty))
	}
	p := tea.NewProgram(model, options...)

	srv := &tuiServer{cfg: cfg, sess: sess, mode: mode, program: p, done: make(chan struct{}), connected: make(chan struct{}), delivered: make(chan struct{})}

	var listener net.Listener
	{
		sock := review.SocketPathFor(sess.Key)
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			return nil, fmt.Errorf("creating sessions dir: %w", err)
		}
		ln, err := ipc.Listen(sock)
		if err != nil {
			return nil, err
		}
		listener = ln
		defer func() {
			if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				fmt.Fprintf(os.Stderr, "critty: closing review socket: %v\n", err)
			}
			if err := os.Remove(sock); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "critty: removing review socket: %v\n", err)
			}
		}()

		sess.Meta.PID = os.Getpid()
		sess.Meta.SocketPath = sock
		if err := sess.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "critty: warning: could not register session: %v\n", err)
		}

		go srv.acceptLoop(ln)
		if serving {
			select {
			case <-srv.connected:
			case <-time.After(20 * time.Second):
				return nil, fmt.Errorf("review client did not connect")
			}
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range finishCh {
			srv.handleFinish(ev.Approved, ev.Result)
		}
	}()

	finalModel, runErr := p.Run()
	switch app := finalModel.(type) {
	case tui.AppModel:
		if err := app.SaveUnsubmittedDraft(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	case *tui.AppModel:
		if err := app.SaveUnsubmittedDraft(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	close(finishCh)
	<-done
	if err := listener.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "critty: closing review socket: %v\n", err)
	}
	close(srv.done)
	hasClient := serving
	select {
	case <-srv.connected:
		hasClient = true
	default:
	}
	if hasClient {
		select {
		case <-srv.delivered:
		case <-time.After(5 * time.Second):
		}
	}
	if runErr != nil {
		return nil, fmt.Errorf("TUI error: %w", runErr)
	}
	return srv.lastPayload, nil
}

// tuiServer serves one round.  Results are delivered only after the TUI exits.
type tuiServer struct {
	cfg          *config.Config
	sess         *review.Session
	mode         *reviewMode
	program      *tea.Program
	mu           sync.Mutex
	lastPayload  *ipc.FinishPayload
	done         chan struct{}
	connected    chan struct{}
	delivered    chan struct{}
	connectOnce  sync.Once
	deliverOnce  sync.Once
	stopRequests sync.WaitGroup
}

func (s *tuiServer) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *tuiServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	req, err := ipc.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	if req.Type == "stop" {
		s.stopRequests.Add(1)
		defer s.stopRequests.Done()
		s.connectOnce.Do(func() { close(s.connected) })
		s.program.Quit()
		<-s.done
		if err := ipc.WriteMessage(conn, ipc.FinishPayload{Type: "stopped"}); err != nil {
			fmt.Fprintf(os.Stderr, "critty: stopping review: %v\n", err)
		}
		s.deliverOnce.Do(func() { close(s.delivered) })
		return
	}
	if req.Type != "review-cycle" {
		return
	}
	s.connectOnce.Do(func() { close(s.connected) })
	// Losing the blocking client also ends its TUI, preserving saved state.
	go func() {
		var b [1]byte
		if _, err := conn.Read(b[:]); err != nil {
			select {
			case <-s.done:
			default:
				s.program.Quit()
			}
		}
	}()
	<-s.done
	s.stopRequests.Wait()
	s.mu.Lock()
	payload := s.lastPayload
	s.mu.Unlock()
	if payload != nil {
		if err := ipc.WriteMessage(conn, payload); err != nil {
			fmt.Fprintf(os.Stderr, "critty: sending review result: %v\n", err)
		}
	}
	s.deliverOnce.Do(func() { close(s.delivered) })
}

func (s *tuiServer) handleFinish(approved bool, result *review.FormResult) {
	payload := buildFinishPayload(s.cfg, s.sess, s.mode, approved)
	if result != nil {
		payload.Result = result
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			payload.Approved = false
			payload.Prompt += "\nForm result could not be encoded: " + err.Error()
		} else {
			payload.Prompt += "\n\nInteractive form result (version 1):\n" + string(data)
			payload.Prompt += "\nAnswers are human drafts until approved. Only consents explicitly true in an approved result allow their described operations, for this exact review_revision and reviewed_target. Approval alone grants no individual consent. Missing, false, stopped, or feedback-only consent grants no permission. IDs are identifiers, never commands; Critty executes no operations. Preserve this self-contained result before clearing the session."
			if approved && s.cfg.CleanupOnApprove {
				payload.Prompt += fmt.Sprintf("\nAfter recording the result, run `critty clear --session %s` to delete the completed review. Form review cleanup is deferred.", s.sess.Key)
			}
		}
	}
	s.mu.Lock()
	s.lastPayload = &payload
	s.mu.Unlock()
}

// buildFinishPayload assembles the agent-facing finish result, rendering
// the prompt through the template chain.
func buildFinishPayload(cfg *config.Config, sess *review.Session, mode *reviewMode, approved bool) ipc.FinishPayload {
	unresolved := sess.CJ.ListComments(true)
	all := sess.CJ.ListComments(false)

	unresolvedJSON := ""
	if len(unresolved) > 0 {
		if data, err := review.EncodeCommentsJSON(unresolved); err == nil {
			unresolvedJSON = string(data)
		}
	}
	allJSON := ""
	if data, err := review.EncodeCommentsJSON(all); err == nil {
		allJSON = string(data)
	}

	seen := map[string]bool{}
	var filesWithComments []string
	for _, c := range unresolved {
		if c.Path != nil && !seen[*c.Path] {
			seen[*c.Path] = true
			filesWithComments = append(filesWithComments, *c.Path)
		}
	}

	nextCmd := ""
	if !approved {
		nextCmd = nextRoundCommand(sess, mode)
	}

	ctx := prompt.Context{
		ReviewPath:        sess.Path(),
		SessionKey:        sess.Key,
		Mode:              mode.promptMode(),
		InternalMode:      mode.internalMode(),
		PlanSlug:          mode.planSlug,
		UnresolvedCount:   len(unresolved),
		TotalCount:        len(all),
		FilesWithComments: filesWithComments,
		UnresolvedJSON:    unresolvedJSON,
		CommentsJSON:      allJSON,
		Approved:          approved,
		NextRoundCmd:      nextCmd,
	}

	verdict := sess.CJ.RoundState.Verdict
	if !verdict.Valid() {
		verdict = review.VerdictComment
		if approved {
			verdict = review.VerdictApprove
		}
	}
	comment := sess.SubmissionComment()
	finishPrompt := prompt.RenderFinish(cfg.Prompts, cfg.ProjectRoot, ctx)
	finishPrompt += fmt.Sprintf("\n\nReview verdict: %s.", verdict)
	if verdict == review.VerdictReject {
		finishPrompt += " The reviewer requests changes. Address the feedback and return for another review."
	}
	if comment != nil {
		finishPrompt += "\n\nOverall review comment:\n" + comment.Body
	}
	if mode.code() && len(mode.files) == 0 {
		finishPrompt += "\n\nThe selected review scope contains no file changes."
	}
	if documents, err := sess.LoadDocuments(); err == nil && len(documents.Documents) > 0 {
		finishPrompt += "\n\nAttached documents (comment paths identify snapshots, not working files):"
		for _, d := range documents.Documents {
			state := ""
			if d.Removed {
				state = " (removed from the review)"
			}
			finishPrompt += fmt.Sprintf("\n- %q → %q (%s)%s\n%s", d.Path(), d.Source, d.Revision, state, d.Content)
		}
		finishPrompt += "\nTo revise attachments, pass the complete ordered list with repeated --attachment flags when resuming the session. Without these flags, saved snapshots are reused. Use --clear-attachments to remove the list."
	}
	if sess.HasAttachments() {
		finishPrompt += fmt.Sprintf("\n\nImage attachments: resolve Markdown attachments/ paths relative to %q. Read referenced images with your image-viewing tool, including images in resolved comments and replies.", sess.Dir)
		if approved && cfg.CleanupOnApprove {
			finishPrompt += fmt.Sprintf("\nAfter reading the images and recording any remaining instructions, run `critty clear --session %s` from the original working directory to delete this completed review and its images. Approval cleanup is deferred until this command; do not leave the images behind.", sess.Key)
		}
	}
	return ipc.FinishPayload{
		Type:          "finish",
		Verdict:       verdict,
		ReviewComment: comment,
		Approved:      approved,
		Prompt:        finishPrompt,
		Comments:      all,
		NextCommand:   nextCmd,
	}
}

// nextRoundCommand targets the saved review and, when needed, replacement input.
func nextRoundCommand(sess *review.Session, mode *reviewMode) string {
	if mode.patch != nil && len(mode.files) > 0 {
		return "critty --diff --session " + sess.Key + " < updated.diff"
	}
	if mode.plan() && mode.planFile != "" {
		return "critty plan --session " + sess.Key + " " + shellEscape(mode.planFile)
	}
	return "critty --session " + sess.Key
}
