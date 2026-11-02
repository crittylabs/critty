package terminalbinding

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHCSocketPathPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"HC_TERMINAL_SOCKET", "HC_SOCKET", "HC_SESSION", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(key, "")
	}
	if got, want := HCSocketPath(), filepath.Join(home, ".local", "state", "herdr-core", "default", "default.sock"); got != want && !filepath.IsAbs(got) {
		t.Fatalf("default socket = %q", got)
	}
	t.Setenv("HC_SESSION", " work ")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
	if got, want := HCSocketPath(), "/run/user/501/herdr-core/work.sock"; got != want {
		t.Fatalf("runtime socket = %q, want %q", got, want)
	}
	config := filepath.Join(home, "cfg")
	if err := os.MkdirAll(filepath.Join(config, "herdr-core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "herdr-core", "config.toml"), []byte("socket = \"/custom/core.sock\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	if got := HCSocketPath(); got != "/custom/core.sock" {
		t.Fatalf("config socket = %q", got)
	}
	t.Setenv("HC_SOCKET", "/override/core.sock")
	if got := HCSocketPath(); got != "/override/core.sock" {
		t.Fatalf("HC_SOCKET = %q", got)
	}
	t.Setenv("HC_TERMINAL_SOCKET", "/terminal/core.sock")
	if got := HCSocketPath(); got != "/terminal/core.sock" {
		t.Fatalf("HC_TERMINAL_SOCKET = %q", got)
	}
}

func TestHCProcessTargets(t *testing.T) {
	reply := `{"jsonrpc":"2.0","id":1,"result":{"terminals":[
		{"address":"t1","state":"live","pid":100,"foreground":{"pid":110,"name":"claude"},"labels":{"herdr.space":"sp1","herdr.tab":"tb1"}},
		{"address":"t2","state":"finished","pid":200,"foreground":null},
		{"address":"box/t3","state":"live","pid":300,"foreground":{"pid":300}},
		{"address":"t4","state":"live","pid":null,"foreground":{"pid":110}}
	],"hidden_finished":0}}` + "\n"
	terminals, err := DecodeHCTerminals([]byte(reply))
	if err != nil {
		t.Fatal(err)
	}
	owners := HCProcessTargets(terminals, "/core.sock")
	want := Target{Kind: "herdr-app", Socket: "/core.sock", Pane: "t1"}
	if len(owners) != 2 || owners[100] != want || owners[110] != want {
		t.Fatalf("owners = %+v", owners)
	}
	if _, err := DecodeHCTerminals([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"no core"}}`)); err == nil {
		t.Fatal("accepted an error reply")
	}
	if _, err := DecodeHCTerminals([]byte(`not json`)); err == nil {
		t.Fatal("accepted malformed output")
	}
}

func TestHerdrAppTargetValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target Target
		valid  bool
	}{
		{"terminal", Target{Kind: "herdr-app", Socket: "/core.sock", Pane: "t7"}, true},
		{"relative socket", Target{Kind: "herdr-app", Socket: "core.sock", Pane: "t7"}, false},
		{"missing terminal", Target{Kind: "herdr-app", Socket: "/core.sock"}, false},
		{"option-like terminal", Target{Kind: "herdr-app", Socket: "/core.sock", Pane: "--all"}, false},
		{"remote terminal", Target{Kind: "herdr-app", Socket: "/core.sock", Pane: "box/t7"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.target.Validate(); (err == nil) != tc.valid {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestHerdrAppEnvironmentAndSnapshot(t *testing.T) {
	clearTerminalEnv(t)
	t.Setenv("HC_TERMINAL_ID", "t7")
	t.Setenv("HC_TERMINAL_SOCKET", "/right/core.sock")
	t.Setenv("HC_SOCKET", "/wrong/core.sock")
	target, err := FromEnvironment()
	if err != nil || target != (Target{Kind: "herdr-app", Socket: "/right/core.sock", Pane: "t7"}) {
		t.Fatalf("%+v %v", target, err)
	}

	dir := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf '%s' "$HC_SOCKET" > "$SOCKET_FILE"
printf '{"text":"CRITTY-visible","how":"screen"}'
`
	if err := os.WriteFile(filepath.Join(dir, "hc"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("ARGS_FILE", filepath.Join(dir, "args"))
	t.Setenv("SOCKET_FILE", filepath.Join(dir, "socket"))
	text, err := Snapshot(context.Background(), target)
	if err != nil || text != "CRITTY-visible" {
		t.Fatalf("snapshot %q %v", text, err)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "read\n--json\nt7\n" {
		t.Fatalf("args %q", args)
	}
	socket, err := os.ReadFile(filepath.Join(dir, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	if string(socket) != "/right/core.sock" {
		t.Fatalf("socket %q", socket)
	}
}
