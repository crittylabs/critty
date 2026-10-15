package terminalbinding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/crittylabs/critty/internal/xdg"
	"github.com/gofrs/flock"
)

const requestLifetime = 2 * time.Minute
const markerPrefix = "CRITTY-"

// Request is the prepare response. Marker must be displayed verbatim before resolving ID.
type Request struct {
	ID        string    `json:"id"`
	Marker    string    `json:"marker"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pending struct {
	Request
	Notifications []Target  `json:"notifications,omitempty"`
	NotifiedAt    time.Time `json:"notified_at,omitempty"`
}

type registration struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
	Target  Target `json:"target"`
}

type Store struct {
	dir   string
	stamp func(int) (string, error)
}

func DefaultStore() *Store {
	return &Store{dir: filepath.Join(xdg.StateHome(), "terminals"), stamp: processStamp}
}

func processStamp(pid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	stamp := strings.TrimSpace(string(out))
	if stamp == "" {
		return "", fmt.Errorf("process %d is not running", pid)
	}
	return stamp, nil
}

func (s *Store) locked(fn func() error) (err error) {
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.dir, "store.lock"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("terminal registry is busy")
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return fn()
}

func (s *Store) write(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// All readers hold store.lock, so a partial file is never observed concurrently.
	return os.WriteFile(filepath.Join(s.dir, name), b, 0600)
}

// Register records only a candidate; the marker is still needed to attribute a review.
func (s *Store) Register(target Target) error {
	if err := target.Validate(); err != nil {
		return err
	}
	pid := os.Getpid()
	stamp, err := s.stamp(pid)
	if err != nil {
		return err
	}
	return s.locked(func() error { return s.write("launcher-"+strconv.Itoa(pid)+".json", registration{pid, stamp, target}) })
}

func (s *Store) candidates() ([]Target, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var targets []Target
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "launcher-") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var r registration
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(b, &r) != nil || r.PID < 2 || r.Target.Validate() != nil {
			continue
		}
		stamp, err := s.stamp(r.PID)
		if err != nil || stamp != r.Started {
			if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil {
				return nil, err
			}
			continue
		}
		if r.Target.Kind != "terminal" {
			targets = addTarget(targets, r.Target)
		}
	}
	return targets, nil
}

func validID(id string) bool {
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *Store) read(id string) (pending, error) {
	var p pending
	if !validID(id) {
		return p, fmt.Errorf("invalid terminal request ID")
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "request-"+id+".json"))
	if err != nil {
		return p, fmt.Errorf("terminal request unavailable; prepare a new marker: %w", err)
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.ID != id || p.Marker != markerPrefix+id {
		return p, fmt.Errorf("invalid terminal request")
	}
	if time.Now().After(p.ExpiresAt) {
		return p, fmt.Errorf("terminal request expired; prepare a new marker")
	}
	return p, nil
}

func (s *Store) Prepare() (Request, error) {
	var request Request
	err := s.locked(func() error {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "request-") || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return err
			}
			if time.Since(info.ModTime()) > requestLifetime {
				if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil {
					return err
				}
			}
		}
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return err
		}
		id := hex.EncodeToString(bytes[:])
		request = Request{ID: id, Marker: markerPrefix + id, ExpiresAt: time.Now().Add(requestLifetime)}
		return s.write("request-"+id+".json", pending{Request: request})
	})
	return request, err
}

// Notify is the tfil intake. Multiple distinct destinations remain ambiguous.
func (s *Store) Notify(id string, target Target) error {
	if err := target.Validate(); err != nil {
		return err
	}
	return s.locked(func() error {
		p, err := s.read(id)
		if err != nil {
			return err
		}
		before := len(p.Notifications)
		p.Notifications = addTarget(p.Notifications, target)
		if len(p.Notifications) != before {
			p.NotifiedAt = time.Now()
		}
		return s.write("request-"+id+".json", p)
	})
}

// Resolve prefers notifications and otherwise scans only live registrations' visible screens.
// It consumes the request once a destination is selected, before the caller launches its UI.
func (s *Store) Resolve(ctx context.Context, id string, snapshot func(context.Context, Target) (string, error)) (Target, error) {
	return s.resolve(ctx, id, snapshot, false)
}

// ResolveNotification tests the terminal filter callback without falling back to
// registered pane screens. It consumes the request only after a notification.
func (s *Store) ResolveNotification(ctx context.Context, id string) (Target, error) {
	return s.resolve(ctx, id, nil, true)
}

func (s *Store) resolve(ctx context.Context, id string, snapshot func(context.Context, Target) (string, error), notifyOnly bool) (Target, error) {
	// Let a notification arriving with the just-rendered marker reach the intake first.
	if err := pause(ctx, 150*time.Millisecond); err != nil {
		return Target{}, err
	}
	var lastErr error
	for {
		var p pending
		var targets []Target
		err := s.locked(func() error {
			var err error
			p, err = s.read(id)
			if err != nil {
				return err
			}
			if len(p.Notifications) == 0 && !notifyOnly {
				targets, err = s.candidates()
			}
			return err
		})
		if err != nil {
			return Target{}, err
		}
		matches := p.Notifications
		var scanErr error
		if len(matches) == 0 {
			for _, t := range targets {
				text, err := snapshot(ctx, t)
				if err != nil {
					lastErr = err
					scanErr = err
					continue
				}
				if strings.Contains(text, p.Marker) {
					matches = append(matches, t)
				}
			}
		}
		var selected Target
		// Recheck intake under the same lock as consumption: notify cannot race a stale read.
		err = s.locked(func() error {
			latest, err := s.read(id)
			if err != nil {
				return err
			}
			if len(latest.Notifications) > 0 {
				matches = latest.Notifications
				if time.Since(latest.NotifiedAt) < 100*time.Millisecond {
					return nil
				}
			} else if scanErr != nil {
				// An unread candidate could also display the marker. Do not select a partial match.
				return nil
			}
			if len(matches) > 1 {
				return fmt.Errorf("terminal marker matches multiple panes; leave one displaying this conversation and prepare a new marker")
			}
			if len(matches) == 1 {
				selected = matches[0]
				return os.Remove(filepath.Join(s.dir, "request-"+id+".json"))
			}
			return nil
		})
		if err != nil {
			return Target{}, err
		}
		if selected.Kind != "" {
			return selected, nil
		}
		if err := pause(ctx, 100*time.Millisecond); err != nil {
			if notifyOnly {
				return Target{}, fmt.Errorf("no terminal filter notification received; check tfil --critty-notify and its Critty executable and XDG_STATE_HOME")
			}
			if lastErr != nil {
				return Target{}, fmt.Errorf("cannot read registered terminal screens (check socket access): %w", lastErr)
			}
			return Target{}, fmt.Errorf("terminal marker not found in registered visible panes; launch through critty codex or a notifying filter, display a new marker, and retry")
		}
	}
}

func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
