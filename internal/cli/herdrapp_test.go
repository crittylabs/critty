package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const herdrAppAgentRecord = `{"address":"t3","state":"live","pid":300,"foreground":{"pid":310},"labels":{"herdr.space":"sp1","herdr.tab":"tb1"}}`

func TestSpawnTUIHerdrAppTab(t *testing.T) {
	origOutput, origLook, origResolve := commandOutput, lookPath, resolveExec
	t.Cleanup(func() { commandOutput, lookPath, resolveExec = origOutput, origLook, origResolve })
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	resolveExec = func() (string, error) { return "/path with spaces/critty", nil }
	t.Setenv("HC_SOCKET", "/wrong/core.sock")
	t.Setenv("XDG_STATE_HOME", "/tmp/state with spaces")
	for _, tc := range []struct {
		name, record, created string
		failFocus             bool
		wantCalls             []string
		wantError             string
	}{
		{"success", herdrAppAgentRecord, `{"space":"sp1","tab":"tb9","terminal":"t9"}`, false, []string{"get", "tab.new", "pane.focus"}, ""},
		{"focus failure", herdrAppAgentRecord, `{"space":"sp1","tab":"tb9","terminal":"t9"}`, true, []string{"get", "tab.new", "pane.focus", "tab.close", "tab.select", "pane.focus"}, "focusing Herdr tab"},
		{"hidden terminal", `{"address":"t3","state":"live","labels":{}}`, "", false, []string{"get"}, "not shown in a Herdr window"},
		{"window error", herdrAppAgentRecord, `{"error":"no space \"sp1\""}`, false, []string{"get", "tab.new"}, `no space "sp1"`},
		{"bad create reply", herdrAppAgentRecord, `{"space":"sp1"}`, false, []string{"get", "tab.new"}, "missing tab or terminal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				switch cmd.Path {
				case "/bin/hc":
					if !slices.Contains(cmd.Env, "HC_SOCKET=/right/core.sock") || slices.Contains(cmd.Env, "HC_SOCKET=/wrong/core.sock") {
						t.Fatalf("socket not pinned: %v", cmd.Env)
					}
					calls = append(calls, cmd.Args[1])
					if !slices.Equal(cmd.Args[1:], []string{"get", "--json", "t3"}) {
						t.Fatalf("unexpected hc command: %v", cmd.Args)
					}
					return []byte(tc.record), nil
				case "/bin/herdr-app":
					if cmd.Args[1] != "do" || len(cmd.Args) != 4 {
						t.Fatalf("unexpected herdr-app command: %v", cmd.Args)
					}
					calls = append(calls, cmd.Args[2])
					var args map[string]any
					if err := json.Unmarshal([]byte(cmd.Args[3]), &args); err != nil {
						t.Fatalf("arguments are not JSON: %v", cmd.Args)
					}
					switch cmd.Args[2] {
					case "tab.new":
						if args["space"] != "sp1" || args["name"] != "Critty" || args["folder"] == "" {
							t.Fatalf("tab.new arguments: %v", args)
						}
						argv, _ := args["argv"].([]any)
						if len(argv) != 3 || argv[0] != "/bin/sh" || argv[1] != "-c" {
							t.Fatalf("tab.new argv: %v", args["argv"])
						}
						command, _ := argv[2].(string)
						for _, want := range []string{"exec env -u NO_COLOR CRITTY_HOST='Herdr'", "XDG_STATE_HOME='/tmp/state with spaces'", "'/path with spaces/critty' _tui --session '0123456789ab'"} {
							if !strings.Contains(command, want) {
								t.Fatalf("incorrect TUI command: %s", command)
							}
						}
						return []byte(tc.created), nil
					case "pane.focus":
						if tc.failFocus && args["terminal"] == "t9" {
							return []byte(`{"error":"no such pane"}`), fmt.Errorf("exit status 1")
						}
						if args["terminal"] != "t9" && args["terminal"] != "t3" {
							t.Fatalf("pane.focus arguments: %v", args)
						}
						return []byte(`{}`), nil
					case "tab.close":
						if args["id"] != "tb9" || args["confirmed"] != true {
							t.Fatalf("tab.close arguments: %v", args)
						}
						return []byte(`{}`), nil
					case "tab.select":
						if args["id"] != "tb1" {
							t.Fatalf("tab.select arguments: %v", args)
						}
						return []byte(`{}`), nil
					}
				}
				return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
			}
			launch, err := spawnTUIHerdrAppTab(&reviewMode{sessionKey: "0123456789ab"}, herdrAppContext{socket: "/right/core.sock", terminal: "t3"})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil || launch.tab != "tb9" || launch.sourceTab != "tb1" {
				t.Fatalf("launch = %+v, %v", launch, err)
			}
			if !slices.Equal(calls, tc.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}

func TestHerdrAppCloseHandlesVanishedTab(t *testing.T) {
	origOutput := commandOutput
	t.Cleanup(func() { commandOutput = origOutput })
	for _, tc := range []struct {
		name, world string
		wantError   bool
	}{
		{"already closed", `{"spaces":[{"id":"sp1","tabs":[{"id":"tb1"}]}]}`, false},
		{"still exists", `{"spaces":[{"id":"sp1","tabs":[{"id":"tb1"},{"id":"tb9"}]}]}`, true},
		{"unreadable", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Args[1] == "ls" {
					if tc.world == "" {
						return nil, fmt.Errorf("no window")
					}
					return []byte(tc.world), nil
				}
				return []byte(`{"error":"no tab \"tb9\""}`), fmt.Errorf("exit status 1")
			}
			err := (herdrAppLaunch{context: herdrAppContext{socket: "/core.sock", terminal: "t3"}, bin: "/bin/herdr-app", tab: "tb9", sourceTab: "tb1"}).close()
			if (err != nil) != tc.wantError {
				t.Fatalf("close = %v", err)
			}
		})
	}
}

func TestHerdrAppDetectorAndEnvironment(t *testing.T) {
	t.Setenv("HC_TERMINAL_ID", "t3")
	t.Setenv("HC_TERMINAL_SOCKET", "/right/core.sock")
	c := (herdrAppDetector{}).environmentContext()
	want := herdrAppContext{socket: "/right/core.sock", terminal: "t3"}
	if c != want {
		t.Fatalf("context = %+v", c)
	}
	vars, err := environmentForTerminal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []envVar{{"HC_TERMINAL_ID", "t3"}, {"HC_TERMINAL_SOCKET", "/right/core.sock"}} {
		if !slices.Contains(vars, v) {
			t.Errorf("missing %v in %v", v, vars)
		}
	}
	t.Setenv("HC_TERMINAL_ID", "")
	if (herdrAppDetector{}).environmentContext() != nil {
		t.Fatal("accepted missing terminal")
	}

	origOutput, origLook := commandOutput, lookPath
	t.Cleanup(func() { commandOutput, lookPath = origOutput, origLook })
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Path != "/bin/hc" || cmd.Args[1] != "connect" || !slices.Contains(cmd.Env, "HC_SOCKET=/right/core.sock") {
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		}
		return []byte(`{"jsonrpc":"2.0","id":1,"result":{"terminals":[` + herdrAppAgentRecord + `]}}` + "\n"), nil
	}
	contexts := (herdrAppDetector{}).processContexts()
	if len(contexts) != 2 || contexts[300] != want || contexts[310] != want {
		t.Fatalf("process contexts = %+v", contexts)
	}
}

func TestHerdrAppLauncherRequiresWindow(t *testing.T) {
	origOutput, origLook := commandOutput, lookPath
	t.Cleanup(func() { commandOutput, lookPath = origOutput, origLook })
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil }
	for _, tc := range []struct {
		name, record string
		want         string
	}{
		{"shown", herdrAppAgentRecord, "herdr-app"},
		{"hidden", `{"address":"t3","state":"live","labels":{}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) { return []byte(tc.record), nil }
			mode := &reviewMode{sessionKey: "0123456789ab"}
			got, err := selectReviewLauncher([]string{"mux"}, herdrAppContext{socket: "/core.sock", terminal: "t3"}, mode)
			if tc.want == "" {
				// A hidden terminal falls through to the desktop defaults with its reason recorded.
				if err != nil || got == nil || got.name == "herdr-app" {
					t.Fatalf("launcher = %+v, %v", got, err)
				}
				if !slices.ContainsFunc(mode.launch.Failures, func(f launcherFailure) bool {
					return f.Launcher == "herdr-app" && strings.Contains(f.Reason, "not shown in a Herdr window")
				}) {
					t.Fatalf("failures = %+v", mode.launch.Failures)
				}
				return
			}
			if err != nil || got == nil || got.name != tc.want {
				t.Fatalf("launcher = %+v, %v", got, err)
			}
		})
	}
}

func TestSpawnTUIHerdrCoreTab(t *testing.T) {
	origOutput, origLook, origResolve := commandOutput, lookPath, resolveExec
	t.Cleanup(func() { commandOutput, lookPath, resolveExec = origOutput, origLook, origResolve })
	lookPath = func(name string) (string, error) {
		if name == "herdr-app" {
			return "", fmt.Errorf("%s not installed", name)
		}
		return "/bin/" + name, nil
	}
	resolveExec = func() (string, error) { return "/usr/local/bin/critty", nil }
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	t.Setenv("HC_TERMINAL_ID", "t3")
	for _, tc := range []struct {
		name, reply, wantError string
	}{
		{"success", `{"address":"t9","state":"live"}`, ""},
		{"no address", `{"error":"no such space"}`, "missing terminal address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Path != "/bin/hc" {
					t.Fatalf("unexpected command: %v", cmd.Args)
				}
				calls = append(calls, cmd.Args[1])
				switch cmd.Args[1] {
				case "get":
					return []byte(herdrAppAgentRecord), nil
				case "run":
					args := cmd.Args[2:]
					sep := slices.Index(args, "--")
					if sep < 0 {
						t.Fatalf("missing argv separator: %v", args)
					}
					flags, argv := args[:sep], args[sep+1:]
					for _, want := range []string{"--json", "--bg", "--no-notify", "--no-parent", "--rm"} {
						if !slices.Contains(flags, want) {
							t.Fatalf("hc run flags: %v", flags)
						}
					}
					if i := slices.Index(flags, "--label"); i < 0 || flags[i+1] != "herdr.space=sp1" {
						t.Fatalf("hc run labels: %v", flags)
					}
					if i := slices.Index(flags, "--cwd"); i < 0 || flags[i+1] == "" {
						t.Fatalf("hc run cwd: %v", flags)
					}
					if len(argv) != 3 || argv[0] != "/bin/sh" || argv[1] != "-c" || !strings.Contains(argv[2], "exec env -u NO_COLOR CRITTY_HOST='Herdr'") {
						t.Fatalf("hc run argv: %v", argv)
					}
					for _, entry := range cmd.Env {
						if strings.HasPrefix(entry, "TMUX=") || strings.HasPrefix(entry, "HC_TERMINAL_") {
							t.Fatalf("stale terminal variable passed: %s", entry)
						}
					}
					if !slices.Contains(cmd.Env, "HC_SOCKET=/core.sock") {
						t.Fatalf("socket not pinned: %v", cmd.Env)
					}
					return []byte(tc.reply), nil
				}
				return nil, fmt.Errorf("unexpected hc command: %v", cmd.Args)
			}
			launch, err := spawnTUIHerdrAppTab(&reviewMode{sessionKey: "0123456789ab"}, herdrAppContext{socket: "/core.sock", terminal: "t3"})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || launch.bin != "" || launch.terminal != "t9" || launch.space != "sp1" {
				t.Fatalf("launch = %+v, %v", launch, err)
			}
			if !slices.Equal(calls, []string{"get", "run"}) {
				t.Fatalf("calls = %v", calls)
			}
		})
	}
}

func TestHerdrCoreCloseHandlesEndedTerminal(t *testing.T) {
	origOutput := commandOutput
	t.Cleanup(func() { commandOutput = origOutput })
	for _, tc := range []struct {
		name              string
		killFails, exists bool
		wantError         bool
	}{
		{"killed", false, true, false},
		{"already ended", true, false, false},
		{"still running", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
				calls = append(calls, cmd.Args[1])
				if cmd.Path != "/bin/hc" || cmd.Args[len(cmd.Args)-1] != "t9" {
					t.Fatalf("unexpected command: %v", cmd.Args)
				}
				switch cmd.Args[1] {
				case "kill":
					if tc.killFails {
						return nil, fmt.Errorf("exit status 1")
					}
				case "get":
					if !tc.exists {
						return nil, fmt.Errorf("exit status 1")
					}
				}
				return []byte(`{}`), nil
			}
			launch := herdrAppLaunch{context: herdrAppContext{socket: "/core.sock", terminal: "t3"}, core: "/bin/hc", terminal: "t9"}
			if err := launch.close(); (err != nil) != tc.wantError {
				t.Fatalf("close = %v (calls %v)", err, calls)
			}
			launch.restoreFocus()
			if slices.Contains(calls, "do") {
				t.Fatal("window command used without herdr-app")
			}
		})
	}
}
