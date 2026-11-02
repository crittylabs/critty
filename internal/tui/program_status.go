package tui

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Critty reports that a review is waiting through the Program Status
// Protocol (OSC 7501), so terminals and agent inboxes that read it, such as
// the Herdr desktop app, cmux, iTerm2, and kitty, show the review as needing
// attention.  Terminals that do not know the sequence ignore it.

const (
	programStatusApp      = "critty"
	programStatusTitleMax = 192
	programStatusMsgMax   = 2048
)

func programStatusSequence(pairs ...string) string {
	return "\x1b]7501;" + strings.Join(pairs, ":") + "\x1b\\"
}

// programStatusText encodes free text as the protocol requires: UTF-8 without
// control characters, within the record's byte limit, as standard base64.
func programStatusText(text string, limit int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(text))
	for len(text) > limit {
		_, size := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-size]
	}
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func programStatusBlocked(kind, title, msg string) string {
	pairs := []string{"state=blocked", "kind=" + kind, "app=" + programStatusApp}
	if title != "" {
		pairs = append(pairs, "title="+programStatusText(title, programStatusTitleMax))
	}
	if msg != "" {
		pairs = append(pairs, "msg="+programStatusText(msg, programStatusMsgMax))
	}
	return programStatusSequence(pairs...)
}

func programStatusClear() string {
	return programStatusSequence("state=clear")
}

func (m AppModel) programStatusReport() string {
	title := m.project
	if m.branch != "" {
		title += " (" + m.branch + ")"
	}
	msg := "Review requested"
	if m.topic != "" && m.topic != m.branch {
		msg = m.topic
	}
	return programStatusBlocked(m.programStatusKind(), title, msg)
}

// programStatusKind distinguishes an approval request, which carries consent
// items, from a review that asks for comments or answers.
func (m AppModel) programStatusKind() string {
	if m.forms != nil {
		for _, d := range m.forms.Documents {
			for _, item := range d.Items {
				if item.Consent {
					return "permission"
				}
			}
		}
	}
	return "question"
}

// reportProgramStatus tells the terminal a reviewer is needed.  Read-only
// views wait for nothing and report nothing.
func (m AppModel) reportProgramStatus() tea.Cmd {
	if m.readOnly {
		return nil
	}
	return tea.Raw(m.images.wrap(m.programStatusReport()))
}

// ProgramStatusClear removes the record after the review process has left
// the terminal, so a terminal without shell integration does not keep showing
// a finished review as waiting.
func ProgramStatusClear() string {
	return newImageDisplay().wrap(programStatusClear())
}
