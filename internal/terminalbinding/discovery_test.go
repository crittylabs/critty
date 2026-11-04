package terminalbinding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDiscoverUnregisteredMuxPanes(t *testing.T) {
	clearTerminalEnv(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	profile := filepath.Join(dir, "profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"host-installation.json": `{"installationId":"source1"}`,
		"orca-runtime.json":      `{"runtimeId":"runtime1"}`,
	} {
		if err := os.WriteFile(filepath.Join(profile, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ORCA_USER_DATA_PATH", profile)
	t.Setenv("ORCA_WORKTREE_ID", "wrong-workspace")
	t.Setenv("ORCA_RUNTIME_SOURCE_ID", "old-source")
	t.Setenv("ORCA_RUNTIME_SOURCE_INCARNATION", "old-runtime")
	t.Setenv("ORCA_RUNTIME_SOURCE_PROFILE_PATH", "")
	t.Setenv("HERDR_SOCKET_PATH", "/custom/herdr.sock")
	t.Setenv("HC_SOCKET", "/custom/core.sock")
	t.Setenv("CMUX_SOCKET_PATH", "/custom/cmux.sock")
	t.Setenv("TMUX", "/custom/tmux.sock,100,1")
	scripts := map[string]string{
		"tmux":   `printf '/custom/tmux.sock\t%%7\t100\n'`,
		"herdr":  `printf '%s' '{"result":{"panes":[{"workspace_id":"w1","tab_id":"t1","pane_id":"p1"}]}}'`,
		"hc":     `read -r request; printf '%s' '{"result":{"terminals":[{"address":"term1","state":"live","pid":200},{"address":"remote/term2","state":"live"},{"address":"term3","state":"exited"}]}}'`,
		"cmux":   `printf '%s' '{"windows":[{"workspaces":[{"id":"w1","panes":[{"surfaces":[{"id":"s1"}]}]}]}]}'`,
		"zellij": `if [ "$1" = list-sessions ]; then printf 'session1\n'; else printf '%s' '[{"id":1,"is_plugin":false},{"id":2,"is_plugin":true}]'; fi`,
		WorkspaceExecutable("orca"): `if [ "$ORCA_RUNTIME_SOURCE_ID" != source1 ] || [ "$ORCA_RUNTIME_SOURCE_INCARNATION" != runtime1 ]; then exit 1; fi
printf '%s' '{"ok":true,"result":{"terminals":[{"handle":"term1","worktreeId":"w1","executionHostId":"local"},{"handle":"remote1","worktreeId":"w2","executionHostId":"ssh:remote"}],"hostScope":{"hostIds":["local"]},"truncated":false}}'`,
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	registered := Target{Kind: "tmux", Socket: "/custom/tmux.sock", Pane: "%7", Origin: Origin{PID: 300, Started: "client", SSH: true}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	found := discoverTargets(ctx, []Target{registered})
	for _, kind := range []string{"tmux", "herdr", "herdr-app", "cmux", "zellij", "orca"} {
		if !slices.ContainsFunc(found.targets, func(target Target) bool { return target.Kind == kind }) {
			t.Errorf("%s was not discovered: %+v", kind, found)
		}
	}
	tmuxCount := 0
	for _, target := range found.targets {
		if target.Kind == "tmux" {
			tmuxCount++
			if target != registered {
				t.Errorf("lost registered client origin: %+v", target)
			}
		}
		if strings.Contains(target.Pane, "remote") || target.Pane == "term3" || target.Kind == "zellij" && target.Pane == "2" {
			t.Errorf("discovered nonlocal, exited, or plugin pane: %+v", target)
		}
	}
	if tmuxCount != 1 {
		t.Errorf("duplicate tmux candidates: %d", tmuxCount)
	}
}

func TestDiscoveryRejectsIncompleteInventories(t *testing.T) {
	for _, tc := range []struct{ kind, body string }{
		{"tmux", `printf 'bad record\n'`},
		{"herdr", `printf '%s' '{"error":{"message":"unavailable"}}'`},
		{"herdr-app", `printf '%s' '{"error":{"code":1,"message":"denied"}}'`},
		{"herdr-app", `printf '%s' '{}'`},
		{"cmux", `printf '%s' '{}'`},
		{"zellij", `if [ "$1" = list-sessions ]; then printf 'session1\n'; else printf '%s' '[{"id":1}]'; fi`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			name := tc.kind
			if name == "herdr-app" {
				name = "hc"
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+tc.body+"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			found := discoverConnection(context.Background(), Target{Kind: tc.kind, Socket: "/socket"})
			if len(found.targets) != 0 || len(found.issues) != 1 {
				t.Fatalf("accepted incomplete inventory: %+v", found)
			}
		})
	}
}

func TestParallelSearchWaitsForAllCandidatesAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	started := make(chan int, len(inputs))
	done := make(chan []int, 1)
	go func() {
		done <- parallelSearch(ctx, inputs, func(ctx context.Context, n int) int {
			started <- n
			<-ctx.Done()
			return n
		})
	}()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for range 8 {
		select {
		case <-started:
		case <-deadline.C:
			t.Fatal("searches did not run concurrently")
		}
	}
	select {
	case <-started:
		t.Fatal("more than eight searches ran concurrently")
	default:
	}
	select {
	case <-done:
		t.Fatal("returned before all candidates finished")
	default:
	}
	cancel()
	select {
	case results := <-done:
		if !slices.Equal(results, inputs) {
			t.Fatalf("unstable result order: %v", results)
		}
	case <-deadline.C:
		t.Fatal("cancellation did not finish searches")
	}
}

func TestDiscoveryResolutionWithoutRegistration(t *testing.T) {
	t.Parallel()
	s := testStore(t)
	r := prepare(t, s)
	target := testTarget("%1")
	s.discover = func(context.Context, []Target) discovery {
		return discovery{targets: []Target{target}, issues: []DiscoveryIssue{{Kind: "orca", Reason: "CLI unavailable"}}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, issues, err := s.ResolveWithDiagnostics(ctx, r.ID, func(context.Context, Target) (string, error) { return r.Marker, nil })
	if err != nil || got != target || len(issues) != 1 {
		t.Fatalf("resolve = %+v, %+v, %v", got, issues, err)
	}
	// Diagnostics remain structured for callers and contain no screen text.
	if _, err := json.Marshal(issues); err != nil {
		t.Fatal(err)
	}
}
