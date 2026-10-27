package ipc

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crittylabs/critty/internal/review"
)

func TestReviewCycleRoundTrip(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		closeTestSocket(t, ln)
		<-done
	})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Aliveness probes connect and close without a request;
			// keep accepting until a real review-cycle arrives.
			req, err := ReadRequest(bufio.NewReader(conn))
			if err != nil || req.Type != "review-cycle" {
				closeTestSocket(t, conn)
				continue
			}
			if err := WriteMessage(conn, FinishPayload{
				Type:     "finish",
				Approved: false,
				Prompt:   "fix things",
				Comments: []review.ListedComment{{Scope: "line", Comment: review.Comment{ID: "c_1"}}},
			}); err != nil {
				t.Errorf("writing finish payload: %v", err)
			}
			closeTestSocket(t, conn)
			return
		}
	}()

	if !Alive(sock) {
		t.Fatal("expected socket alive")
	}
	payload, err := ReviewCycle(sock)
	if err != nil {
		t.Fatalf("ReviewCycle: %v", err)
	}
	if payload.Approved || payload.Prompt != "fix things" || len(payload.Comments) != 1 {
		t.Errorf("unexpected payload: %+v", payload)
	}
}

func TestReviewCycleServerCloseWithoutFinish(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		closeTestSocket(t, ln)
		<-done
	})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		closeTestSocket(t, conn)
	}()

	if _, err := ReviewCycle(sock); err == nil {
		t.Fatal("expected error when the session closes without finishing")
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	closeTestSocket(t, ln) // leaves the socket file behind on some platforms

	ln2, err := Listen(sock)
	if err != nil {
		t.Fatalf("expected stale socket replacement, got %v", err)
	}
	closeTestSocket(t, ln2)
}

func TestListenRejectsLiveSocket(t *testing.T) {
	sock := shortSockPath(t)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		closeTestSocket(t, ln)
		<-done
	})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			closeTestSocket(t, conn)
		}
	}()

	if _, err := Listen(sock); err == nil {
		t.Fatal("expected error for a live socket")
	}
}

func TestWaitAliveTimesOut(t *testing.T) {
	sock := shortSockPath(t)
	if err := WaitAlive(sock, 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "review session did not start") {
		t.Fatalf("expected startup timeout, got %v", err)
	}
}

// shortSockPath returns a socket path short enough for the platform's
// sun_path limit (t.TempDir embeds long test names).
func shortSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "critty-ipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing socket directory: %v", err)
		}
	})
	return filepath.Join(dir, "s.sock")
}

func closeTestSocket(t *testing.T, socket io.Closer) {
	t.Helper()
	if err := socket.Close(); err != nil {
		t.Errorf("closing socket: %v", err)
	}
}
