package tui

import (
	"encoding/base64"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/review"
)

func TestProgramStatusReport(t *testing.T) {
	m := AppModel{project: "critty", branch: "main", topic: "Prepare v1.9.0"}
	got := m.programStatusReport()
	title := base64.StdEncoding.EncodeToString([]byte("critty (main)"))
	msg := base64.StdEncoding.EncodeToString([]byte("Prepare v1.9.0"))
	want := "\x1b]7501;state=blocked:kind=question:app=critty:title=" + title + ":msg=" + msg + "\x1b\\"
	if got != want {
		t.Fatalf("report = %q, want %q", got, want)
	}
	m.topic = "main"
	if got := m.programStatusReport(); !strings.Contains(got, "msg="+base64.StdEncoding.EncodeToString([]byte("Review requested"))) {
		t.Fatalf("branch topic did not fall back to the default message: %q", got)
	}
	t.Setenv("TMUX", "")
	if got := ProgramStatusClear(); got != "\x1b]7501;state=clear\x1b\\" {
		t.Fatalf("clear = %q", got)
	}
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	if got := ProgramStatusClear(); !strings.HasPrefix(got, "\x1bPtmux;\x1b\x1b]7501;state=clear") {
		t.Fatalf("clear inside tmux = %q", got)
	}
}

func TestProgramStatusKindFollowsConsentItems(t *testing.T) {
	m := AppModel{}
	if got := m.programStatusKind(); got != "question" {
		t.Fatalf("kind without forms = %q", got)
	}
	m.forms = &review.FormState{Documents: []review.FormDocument{{Items: []form.Item{{ID: "note"}}}}}
	if got := m.programStatusKind(); got != "question" {
		t.Fatalf("kind without consent = %q", got)
	}
	m.forms.Documents[0].Items = append(m.forms.Documents[0].Items, form.Item{ID: "push", Consent: true})
	if got := m.programStatusKind(); got != "permission" {
		t.Fatalf("kind with consent = %q", got)
	}
}

func TestProgramStatusTextIsSafeAndBounded(t *testing.T) {
	decoded, err := base64.StdEncoding.DecodeString(programStatusText(" tab\there\x1bnew\nline ", 2048))
	if err != nil || string(decoded) != "tab here new line" {
		t.Fatalf("decoded %q %v", decoded, err)
	}
	long := strings.Repeat("あ", 100)
	decoded, err = base64.StdEncoding.DecodeString(programStatusText(long, 192))
	if err != nil || len(decoded) != 192 || string(decoded) != strings.Repeat("あ", 64) {
		t.Fatalf("truncated %q (%d bytes) %v", decoded, len(decoded), err)
	}
}

func TestProgramStatusRespectsReadOnlyAndTmux(t *testing.T) {
	if (AppModel{readOnly: true}).reportProgramStatus() != nil {
		t.Fatal("read-only view reported a status")
	}
	m := AppModel{project: "critty", images: imageDisplay{tmux: true}}
	msg := m.reportProgramStatus()()
	raw, ok := msg.(tea.RawMsg)
	text, _ := raw.Msg.(string)
	if !ok || !strings.HasPrefix(text, "\x1bPtmux;\x1b\x1b]7501;") {
		t.Fatalf("tmux report = %#v", msg)
	}
}
