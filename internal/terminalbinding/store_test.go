package terminalbinding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return &Store{dir: t.TempDir(), stamp: func(int) (string, error) { return "born", nil }}
}

func testTarget(pane string) Target {
	return Target{Kind: "tmux", Socket: "/tmp/test.sock", Pane: pane}
}

func TestProbeCurrentTerminal(t *testing.T) {
	for _, mode := range []string{"pane", "notification", "wrong notification", "display failure", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			if err := s.Register(testTarget("%99")); err != nil {
				t.Fatal(err)
			}
			target := testTarget("%1")
			if mode == "notification" {
				target = Target{}
			}
			var marker string
			display := func(text string) error {
				marker = text
				if mode == "display failure" {
					return errors.New("display unavailable")
				}
				if mode == "notification" || mode == "wrong notification" {
					return s.Notify(strings.TrimPrefix(text, markerPrefix), testTarget("%2"))
				}
				return nil
			}
			snapshot := func(_ context.Context, current Target) (string, error) {
				if current != target || mode == "notification" {
					t.Fatalf("probe read another terminal: %+v", current)
				}
				if mode == "timeout" {
					return "", nil
				}
				return marker, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
			defer cancel()
			resolved, err := s.Probe(ctx, target, display, snapshot)
			wantError := mode == "wrong notification" || mode == "display failure" || mode == "timeout"
			if (err != nil) != wantError {
				t.Fatalf("probe = %+v, %v; want error %t", resolved, err, wantError)
			}
			if !wantError && resolved.Kind != "tmux" {
				t.Fatalf("probe did not resolve a terminal: %+v", resolved)
			}
			entries, err := os.ReadDir(s.dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "request-") {
					t.Fatalf("probe left a request behind: %s", entry.Name())
				}
			}
		})
	}
}

func prepare(t *testing.T, s *Store) Request {
	t.Helper()
	r, err := s.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func resolve(t *testing.T, s *Store, id string, read func(context.Context, Target) (string, error)) (Target, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	return s.Resolve(ctx, id, read)
}

func TestNotificationSkipsScreensAndConsumesRequest(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	target := testTarget("%1")
	for range 2 {
		if err := s.Notify(r.ID, target); err != nil {
			t.Fatal(err)
		}
	}
	got, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) {
		t.Fatal("notified request scanned screens")
		return "", nil
	})
	if err != nil || got != target {
		t.Fatalf("resolve: %+v %v", got, err)
	}
	if err := s.Notify(r.ID, target); err == nil {
		t.Fatal("consumed request accepted notification")
	}
}

func TestConflictingNotificationsFail(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	for _, pane := range []string{"%1", "%2"} {
		if err := s.Notify(r.ID, testTarget(pane)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) { t.Fatal("scanned despite notification"); return "", nil })
	if err == nil || !strings.Contains(err.Error(), "multiple panes") {
		t.Fatalf("want ambiguity: %v", err)
	}
}

func TestOnlyLiveRegisteredCandidatesAreScanned(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	target := testTarget("%1")
	if err := s.Register(target); err != nil {
		t.Fatal(err)
	}
	if err := s.locked(func() error { return s.write("launcher-42.json", registration{42, "old", testTarget("%2")}) }); err != nil {
		t.Fatal(err)
	}
	r := prepare(t, s)
	calls := 0
	got, err := resolve(t, s, r.ID, func(_ context.Context, tgt Target) (string, error) {
		calls++
		if tgt != target {
			t.Fatalf("scanned stale target: %+v", tgt)
		}
		return "other text\n" + r.Marker + "\n", nil
	})
	if err != nil || got != target || calls != 1 {
		t.Fatalf("resolve %+v %v (%d calls)", got, err, calls)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "launcher-42.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale registration retained: %v", err)
	}
}

func TestVisibleSearchFailures(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing", "multiple", "unreadable", "partial"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			s := testStore(t)
			r := prepare(t, s)
			if err := s.Register(testTarget("%1")); err != nil {
				t.Fatal(err)
			}
			if scenario == "multiple" || scenario == "partial" {
				if err := s.locked(func() error { return s.write("launcher-42.json", registration{42, "born", testTarget("%2")}) }); err != nil {
					t.Fatal(err)
				}
			}
			_, err := resolve(t, s, r.ID, func(_ context.Context, tgt Target) (string, error) {
				if scenario == "unreadable" || scenario == "partial" && tgt.Pane == "%2" {
					return "", errors.New("socket denied")
				}
				if scenario == "missing" {
					return "a previous marker", nil
				}
				return r.Marker, nil
			})
			if err == nil {
				t.Fatal("unsafe match accepted")
			}
		})
	}
}

func TestNotificationDuringScanTakesPrecedence(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	scanTarget := testTarget("%1")
	notified := testTarget("%2")
	if err := s.Register(scanTarget); err != nil {
		t.Fatal(err)
	}
	got, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) {
		if err := s.Notify(r.ID, notified); err != nil {
			t.Fatal(err)
		}
		return r.Marker, nil
	})
	if err != nil || got != notified {
		t.Fatalf("resolve %+v %v", got, err)
	}
}

func TestRequestExpiryValidationAndPermissions(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	info, err := os.Stat(filepath.Join(s.dir, "request-"+r.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v", info.Mode())
	}
	if err := s.locked(func() error {
		return s.write("request-"+r.ID+".json", pending{Request: Request{ID: r.ID, Marker: r.Marker, ExpiresAt: time.Now().Add(-time.Second)}})
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{r.ID, "../escape", strings.Repeat("x", 32)} {
		if err := s.Notify(id, testTarget("%1")); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
}

func TestConcurrentNotificationsPreserveConflicts(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	var wg sync.WaitGroup
	for _, pane := range []string{"%1", "%2"} {
		wg.Go(func() {
			if err := s.Notify(r.ID, testTarget(pane)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := s.locked(func() error {
		p, err := s.read(r.ID)
		if err == nil && len(p.Notifications) != 2 {
			t.Errorf("lost notifications: %v", p.Notifications)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveNotificationRequiresCallback(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	target := testTarget("%1")
	if err := s.Register(target); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if _, err := s.ResolveNotification(ctx, r.ID); err == nil || !strings.Contains(err.Error(), "no terminal filter notification") {
		t.Fatalf("registration satisfied notification-only probe: %v", err)
	}
	// Failure must leave the request available for a real notification.
	if err := s.Notify(r.ID, target); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := s.ResolveNotification(ctx, r.ID)
	if err != nil || got != target {
		t.Fatalf("notification: %+v, %v", got, err)
	}
	if _, err := s.read(r.ID); err == nil {
		t.Fatal("successful notification was not consumed")
	}
}
