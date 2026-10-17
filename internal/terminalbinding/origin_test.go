package terminalbinding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSSHAncestor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		processes map[int]processInfo
		remote    bool
	}{
		{"sshd", map[int]processInfo{20: {10, "codex"}, 10: {2, "sshd"}}, true},
		{"session", map[int]processInfo{20: {10, "tfil"}, 10: {2, "/usr/sbin/sshd-session"}}, true},
		{"outbound client", map[int]processInfo{20: {10, "codex"}, 10: {1, "ssh"}}, false},
		{"cycle", map[int]processInfo{20: {10, "codex"}, 10: {20, "shell"}}, false},
		{"inaccessible", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sshAncestor(20, func(pid int) (processInfo, error) {
				p, ok := tc.processes[pid]
				if !ok {
					return p, errors.New("gone")
				}
				return p, nil
			})
			if got != tc.remote {
				t.Fatalf("remote=%v, want %v", got, tc.remote)
			}
		})
	}
}

func TestTerminalHints(t *testing.T) {
	for _, tc := range []struct {
		vars map[string]string
		want string
	}{
		{map[string]string{"TERMINAL_APP": "Ghostty"}, "ghostty"},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, "iterm"},
		{map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, "terminal-app"},
		{map[string]string{"WT_SESSION": "fixture"}, "windows-terminal"},
		{map[string]string{"TERM": "xterm-256color"}, ""},
	} {
		if got := terminalApplication(func(k string) string { return tc.vars[k] }); got != tc.want {
			t.Fatalf("hints=%v: %q want %q", tc.vars, got, tc.want)
		}
	}
}

func TestMuxlessNotificationAndPrivateOrigin(t *testing.T) {
	s := testStore(t)
	r := prepare(t, s)
	t.Setenv("SSH_CONNECTION", "private-address-fixture")
	t.Setenv("TERMINAL_APP", "Ghostty")
	origin := CaptureOrigin(os.Getpid())
	target := Target{Kind: "terminal", Origin: origin}
	if err := s.Notify(r.ID, target); err != nil {
		t.Fatal(err)
	}
	got, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) { t.Fatal("scanned muxless terminal"); return "", nil })
	if err != nil || got != target || !Remote(got, got.Origin) {
		t.Fatalf("resolve=%+v, %v", got, err)
	}
	// A notification persists only the positive bit, never connection addresses.
	r = prepare(t, s)
	if err := s.Notify(r.ID, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "request-"+r.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-address-fixture") {
		t.Fatal("SSH addresses persisted")
	}
}

func TestTMUXReattachmentOverridesStartupSSH(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$3" in
 display-message) printf '$1\n' ;;
 list-clients) if [ "$SSH_FIXTURE" = multiple ]; then printf '12\n13\n'; else printf '%s\n' "$CLIENT_PID"; fi ;;
 show-environment)
  case "$SSH_FIXTURE:$6" in
   remote:SSH_CONNECTION) printf 'SSH_CONNECTION=fixture\n' ;;
   local:SSH_CONNECTION) printf '%s\n' '-SSH_CONNECTION' ;;
   local:SSH_CLIENT) printf 'SSH_CLIENT=stale-fixture\n' ;;
   *) exit 1 ;;
  esac ;;
 *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CLIENT_PID", strconv.Itoa(os.Getpid()))
	target := testTarget("%1")
	for _, tc := range []struct {
		fixture       string
		startup, want bool
	}{
		{"remote", false, true}, {"local", true, false}, {"unknown", false, false}, {"unknown", true, true}, {"multiple", true, true},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Setenv("SSH_FIXTURE", tc.fixture)
			if got := Remote(target, Origin{SSH: tc.startup}); got != tc.want {
				t.Fatalf("remote=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestNotificationDeduplicatesEnrichedPane(t *testing.T) {
	s := testStore(t)
	r := prepare(t, s)
	target := testTarget("%1")
	if err := s.Notify(r.ID, target); err != nil {
		t.Fatal(err)
	}
	target.Origin = Origin{PID: 10, Started: "born", Terminal: "ghostty", SSH: true}
	if err := s.Notify(r.ID, target); err != nil {
		t.Fatal(err)
	}
	got, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) { t.Fatal("scanned"); return "", nil })
	if err != nil || got.Origin != target.Origin {
		t.Fatalf("resolve=%+v,%v", got, err)
	}
}

func TestMuxlessNotificationDoesNotGuessBetweenClients(t *testing.T) {
	s := testStore(t)
	r := prepare(t, s)
	for _, pid := range []int{20, 30} {
		if err := s.Notify(r.ID, Target{Kind: "terminal", Origin: Origin{PID: pid, Started: "born"}}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := resolve(t, s, r.ID, func(context.Context, Target) (string, error) { t.Fatal("scanned"); return "", nil })
	if err == nil {
		t.Fatal("selected an ambiguous client")
	}
}
