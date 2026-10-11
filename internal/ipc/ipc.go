// Package ipc implements the Unix-socket protocol between the blocking
// critty client (run by an agent) and the TUI process that owns a review
// session.  It replaces crit's HTTP review-cycle API with newline-delimited
// JSON messages.
package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/knu/tcrit/internal/review"
)

// Request is a client-to-TUI message.
type Request struct {
	Type string `json:"type"` // "review-cycle"
}

// FinishPayload is the TUI-to-client message emitted when the human
// finishes a review round.
type FinishPayload struct {
	Result      *review.FormResult     `json:"result,omitempty"`
	Type        string                 `json:"type"` // "finish"
	Approved    bool                   `json:"approved"`
	Prompt      string                 `json:"prompt"`
	Comments    []review.ListedComment `json:"comments"`
	NextCommand string                 `json:"next_command,omitempty"`
}

// Listen opens the session's Unix socket, replacing a stale socket file
// left behind by a dead process.
func Listen(path string) (net.Listener, error) {
	if _, err := os.Stat(path); err == nil {
		if conn, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
			if err := conn.Close(); err != nil {
				return nil, fmt.Errorf("closing review session probe: %w", err)
			}
			return nil, fmt.Errorf("another review session is already listening on %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("removing stale socket: %w", err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", path, err)
	}
	return ln, nil
}

// Alive reports whether a review session is accepting connections on path.
func Alive(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	if err := conn.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "critty: closing review session probe: %v\n", err)
	}
	return true
}

// ReviewCycle connects to a session socket, requests a review cycle, and
// blocks until the human finishes the round.  A closed connection without a
// finish payload reports the session as ended.
func ReviewCycle(path string) (*FinishPayload, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connecting to review session: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := WriteMessage(conn, Request{Type: "review-cycle"}); err != nil {
		return nil, fmt.Errorf("requesting review cycle: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("review session ended without finishing")
	}
	var payload FinishPayload
	if err := json.Unmarshal(line, &payload); err != nil {
		return nil, fmt.Errorf("parsing finish payload: %w", err)
	}
	return &payload, nil
}

// Stop asks the TUI to exit without approving or deleting the review.
func Stop(path string) error {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return fmt.Errorf("connecting to review: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if err := WriteMessage(conn, Request{Type: "stop"}); err != nil {
		return err
	}
	var response FinishPayload
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return fmt.Errorf("stopping review: %w", err)
	}
	if response.Type != "stopped" {
		return fmt.Errorf("unexpected stop response %q", response.Type)
	}
	return nil
}

// WaitAlive polls until the socket accepts connections or the timeout
// elapses.
func WaitAlive(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Alive(path) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("review session did not start within %s", timeout)
}

// WriteMessage sends one newline-delimited JSON message.
func WriteMessage(conn net.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

// ReadRequest reads one client request from a connection.
func ReadRequest(reader *bufio.Reader) (*Request, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
