package codexwrapper

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before", "codex")
	self := filepath.Join(dir, "self", "codex")
	after := filepath.Join(dir, "after", "codex")
	for _, path := range []string{before, self, after} {
		writeExecutable(t, path, "#!/bin/sh\nexit 0\n")
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(filepath.Dir(self), alias); err != nil {
		t.Fatal(err)
	}
	path := strings.Join([]string{filepath.Dir(before), filepath.Dir(self), alias, filepath.Dir(after)}, string(os.PathListSeparator))
	got, err := resolve(path, self)
	if err != nil || got != after {
		t.Fatalf("resolve = %q, %v, want %q", got, err, after)
	}
	if _, err := resolve(filepath.Dir(before), self); err == nil {
		t.Fatal("missing wrapper in PATH must fail")
	}
}

func TestResolveSkipsMiseShims(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "wrapper", "codex")
	shim := filepath.Join(dir, "shims", "codex")
	real := filepath.Join(dir, "real", "codex")
	for _, p := range []string{self, real} {
		writeExecutable(t, p, "#!/bin/sh\nexit 0\n")
	}
	writeExecutable(t, shim, "#!/bin/sh\n# mise generated shim\nexec mise x -- codex\n")
	path := strings.Join([]string{filepath.Dir(self), filepath.Dir(shim), filepath.Dir(real)}, string(os.PathListSeparator))
	got, err := resolve(path, self)
	if err != nil || got != real {
		t.Fatalf("resolve shell shim = %q, %v", got, err)
	}
	if err := os.Remove(shim); err != nil {
		t.Fatal(err)
	}
	mise := filepath.Join(dir, "mise")
	writeExecutable(t, mise, "fake mise executable")
	if err := os.Symlink(mise, shim); err != nil {
		t.Fatal(err)
	}
	got, err = resolve(path, self)
	if err != nil || got != real {
		t.Fatalf("resolve symlink shim = %q, %v", got, err)
	}
}

func TestWrapperProcess(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper", "codex")
	build := exec.Command("go", "build", "-o", wrapper, "../../cmd/codex")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	critty := filepath.Join(dir, "critty")
	build = exec.Command("go", "build", "-o", critty, "../../cmd/critty")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	real := filepath.Join(dir, "real", "codex")
	writeExecutable(t, real, `#!/bin/sh
printf '%s\000' "$@" > "$TEST_LOG"
printf '%s\n' "$TMUX" "$TMUX_PANE" "$PWD" > "$TEST_ENV"
exit 23
`)
	foreign := filepath.Join(dir, "foreign", "codex")
	writeExecutable(t, foreign, `#!/bin/sh
rest=${PATH#*"${0%/*}:"}
IFS=:
for dir in $rest; do
 if [ -x "$dir/codex" ]; then exec "$dir/codex" "$@"; fi
done
exit 127
`)
	for _, key := range []string{"HERDR_ENV", "HERDR_SOCKET_PATH", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("TMUX", "/test/socket,1,0")
	t.Setenv("TMUX_PANE", "%42")
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("TEST_LOG", filepath.Join(dir, "args"))
	t.Setenv("TEST_ENV", filepath.Join(dir, "env"))
	for _, entry := range []string{"wrapper", "critty", "foreign-first", "foreign-last"} {
		for _, args := range [][]string{{"resume", "--last"}, {"--help"}, {"exec", "--", "--unknown", "a prompt"}} {
			t.Run(entry+strings.Join(args, " "), func(t *testing.T) {
				path := filepath.Dir(wrapper) + ":" + filepath.Dir(real)
				exe := wrapper
				input := args
				switch entry {
				case "critty":
					exe = critty
					input = append([]string{"codex"}, args...)
				case "foreign-first":
					exe = foreign
					path = filepath.Dir(foreign) + ":" + path
				case "foreign-last":
					path = filepath.Dir(wrapper) + ":" + filepath.Dir(foreign) + ":" + filepath.Dir(real)
				}
				t.Setenv("PATH", path)
				cmd := exec.Command(exe, input...)
				cmd.Dir = dir
				err := cmd.Run()
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
					t.Fatalf("exit: %v", err)
				}
				b, err := os.ReadFile(os.Getenv("TEST_LOG"))
				if err != nil {
					t.Fatal(err)
				}
				got := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
				if !reflect.DeepEqual(got, args) {
					t.Fatalf("arguments changed: %q != %q", got, args)
				}
				b, err = os.ReadFile(os.Getenv("TEST_ENV"))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != "/test/socket,1,0\n%42\n"+dir+"\n" {
					t.Fatalf("environment/cwd changed: %q", b)
				}
				registration := filepath.Join(dir, "state", "critty", "terminals", fmt.Sprintf("launcher-%d.json", cmd.Process.Pid))
				b, err = os.ReadFile(registration)
				if err != nil {
					t.Fatal(err)
				}
				var saved struct {
					PID    int                           `json:"pid"`
					Target struct{ Socket, Pane string } `json:"target"`
				}
				if err := json.Unmarshal(b, &saved); err != nil || saved.PID != cmd.Process.Pid || saved.Target.Socket != "/test/socket" || saved.Target.Pane != "%42" {
					t.Fatalf("registration: %s (%v)", b, err)
				}
			})
		}
	}
}
