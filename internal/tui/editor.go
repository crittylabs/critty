package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-shellwords"
)

type editorFinishedMsg struct {
	path string
	err  error
}

func (m *AppModel) openExternalEditor() tea.Cmd {
	file, err := os.CreateTemp("", "critty-comment-*.md")
	if err != nil {
		return func() tea.Msg { return errMsg{fmt.Errorf("creating editor file: %w", err)} }
	}
	path := file.Name()
	if _, err := file.WriteString(m.modalTextarea.Value()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{fmt.Errorf("writing editor file: %w", err)} }
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{fmt.Errorf("closing editor file: %w", err)} }
	}

	cmd, err := externalEditorCommand(path)
	if err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	})
}

func externalEditorCommand(path string) (*exec.Cmd, error) {
	return sourceEditorCommand(path, 0)
}

var (
	editorGotoCache    sync.Map
	editorProbeTimeout = 2 * time.Second
)

func sourceEditorCommand(path string, line int) (*exec.Cmd, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	args, err := shellwords.Parse(editor)
	if err != nil {
		return nil, fmt.Errorf("parsing $EDITOR: %w", err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("$EDITOR is empty")
	}
	if line > 0 {
		if editorSupportsGoto(editor, args) {
			args = append(args, "--goto", fmt.Sprintf("%s:%d", path, line))
		} else {
			args = append(args, fmt.Sprintf("+%d", line), path)
		}
	} else {
		args = append(args, path)
	}
	return exec.Command(args[0], args[1:]...), nil
}

func editorSupportsGoto(editor string, args []string) bool {
	if cached, ok := editorGotoCache.Load(editor); ok {
		return cached.(bool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), editorProbeTimeout)
	defer cancel()
	helpArgs := append(append([]string(nil), args[1:]...), "--help")
	cmd := exec.CommandContext(ctx, args[0], helpArgs...)
	cmd.WaitDelay = 200 * time.Millisecond
	output, _ := cmd.CombinedOutput()
	supported := ctx.Err() == nil && helpHasGoto(string(output))
	editorGotoCache.Store(editor, supported)
	return supported
}

func helpHasGoto(help string) bool {
	for _, line := range strings.Split(help, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && strings.TrimSuffix(fields[0], ",") == "-g" {
			fields = fields[1:]
		}
		if len(fields) > 0 && strings.TrimSuffix(fields[0], ",") == "--goto" {
			return true
		}
	}
	return false
}

func (m *AppModel) finishExternalEdit(msg editorFinishedMsg) {
	defer func() { _ = os.Remove(msg.path) }()
	if msg.err != nil {
		m.err = fmt.Errorf("running $EDITOR: %w", msg.err)
		return
	}
	content, err := os.ReadFile(msg.path)
	if err != nil {
		m.err = fmt.Errorf("reading editor file: %w", err)
		return
	}
	m.modalTextarea.SetValue(string(content))
	m.modalFocus = 0
	m.modalTextarea.Focus()
}
