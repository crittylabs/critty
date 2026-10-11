package cli

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/internal/ipc"
)

type lifecycleModel struct{ ready chan struct{} }

func (m lifecycleModel) Init() tea.Cmd                       { close(m.ready); return nil }
func (m lifecycleModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m lifecycleModel) View() tea.View                      { return tea.NewView("") }

func testRoundServer(t *testing.T) *tuiServer {
	t.Helper()
	ready := make(chan struct{})
	p := tea.NewProgram(lifecycleModel{ready}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	s := &tuiServer{program: p, done: make(chan struct{}), connected: make(chan struct{}), delivered: make(chan struct{})}
	go func() { _, _ = p.Run(); close(s.done) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("program did not start")
	}
	t.Cleanup(func() { p.Quit(); <-s.done })
	return s
}

func TestStopRequestEndsTUIWithoutApproval(t *testing.T) {
	s := testRoundServer(t)
	primary, primaryServer := net.Pipe()
	defer func() { _ = primary.Close() }()
	go s.handleConn(primaryServer)
	if err := ipc.WriteMessage(primary, ipc.Request{Type: "review-cycle"}); err != nil {
		t.Fatal(err)
	}
	<-s.connected
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	go s.handleConn(server)
	if err := ipc.WriteMessage(client, ipc.Request{Type: "stop"}); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	<-s.done
	// The stop response is blocked in net.Pipe until this test reads it.
	// The original client must not close its pane before that response.
	primaryEnded := make(chan struct{})
	go func() { var b [1]byte; _, _ = primary.Read(b[:]); close(primaryEnded) }()
	select {
	case <-primaryEnded:
		t.Fatal("primary client ended before stop response")
	default:
	}
	var response ipc.FinishPayload
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "stopped" || response.Approved || s.lastPayload != nil {
		t.Fatalf("stop became approval: %+v", response)
	}
	select {
	case <-s.done:
	default:
		t.Fatal("stop acknowledged before TUI exit")
	}
}

func TestDisconnectedClientEndsTUI(t *testing.T) {
	s := testRoundServer(t)
	client, server := net.Pipe()
	go s.handleConn(server)
	if err := ipc.WriteMessage(client, ipc.Request{Type: "review-cycle"}); err != nil {
		t.Fatal(err)
	}
	<-s.connected
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("abandoned TUI kept running")
	}
	if s.lastPayload != nil {
		t.Fatal("client cancellation approved review")
	}
}

func TestFinishResultWaitsForTUIExit(t *testing.T) {
	s := testRoundServer(t)
	s.lastPayload = &ipc.FinishPayload{Type: "finish", Approved: false, NextCommand: "critty --session 0123456789ab"}
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	go s.handleConn(server)
	if err := ipc.WriteMessage(client, ipc.Request{Type: "review-cycle"}); err != nil {
		t.Fatal(err)
	}
	<-s.connected
	select {
	case <-s.delivered:
		t.Fatal("result delivered while TUI is running")
	default:
	}
	s.program.Quit()
	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var response ipc.FinishPayload
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "finish" || response.Approved {
		t.Fatalf("wrong finish result: %+v", response)
	}
}
