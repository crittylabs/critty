package terminalbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RexExecutable also finds the CLI bundled with the macOS preview app.
func RexExecutable() (string, error) {
	if bin, err := exec.LookPath("rex"); err == nil {
		return bin, nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			for _, app := range []string{"Rex.app", "Rex Beta.app"} {
				bin := filepath.Join(dir, app, "Contents/Helpers/Rex Server.rex/Contents/MacOS/rex")
				if info, err := os.Stat(bin); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
					return bin, nil
				}
			}
		}
	}
	return "", fmt.Errorf("rex CLI not found")
}

func RexEnvironment() Target {
	endpoint := os.Getenv("REX_SERVER")
	if endpoint == "" && (os.Getenv("REX_SESSION") != "" || os.Getenv("REX_BLOCK") != "") {
		endpoint = rexDefaultEndpoint()
	}
	if filepath.IsAbs(endpoint) {
		endpoint = (&url.URL{Scheme: "unix", Path: endpoint}).String()
	}
	return Target{Kind: "rex", Socket: endpoint, Workspace: os.Getenv("REX_SESSION"), Pane: os.Getenv("REX_BLOCK")}
}

func rexDefaultEndpoint() string {
	bin, err := RexExecutable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--autostart=false", "--timeout", "1s", "whoami", "--json").Output()
	if err != nil {
		return ""
	}
	var result struct {
		Endpoint string `json:"endpoint"`
	}
	if json.Unmarshal(out, &result) != nil {
		return ""
	}
	return result.Endpoint
}

func validateRex(t Target) error {
	u, err := url.Parse(t.Socket)
	if err != nil || u.Scheme != "unix" || u.Host != "" || !filepath.IsAbs(u.Path) || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(t.Socket, "\x00\r\n") {
		return fmt.Errorf("Rex reviews require an explicit local Unix server endpoint")
	}
	if !RexID(t.Workspace, "session") || !RexID(t.Pane, "block") {
		return fmt.Errorf("incomplete Rex terminal identity")
	}
	return nil
}

// RexID accepts only full IDs, never labels, indexes, or ambiguous suffixes.
func RexID(id, kind string) bool {
	suffix, ok := strings.CutPrefix(id, kind+":")
	if !ok || len(suffix) != 36 {
		return false
	}
	for i, c := range suffix {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (t Target) rexCommand(ctx context.Context, args ...string) *exec.Cmd {
	bin, err := RexExecutable()
	if err != nil {
		bin = "rex"
	}
	// REX_SERVER alone may fall back to the default server when unavailable.
	cmd := exec.CommandContext(ctx, bin, append([]string{"--server", t.Socket, "--autostart=false", "--timeout", "2s"}, args...)...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "REX_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	return cmd
}

// RexOwners uses only the inherited local endpoint and full terminal identity.
func RexOwners(ctx context.Context, t Target) map[int]Target {
	if t.Validate() != nil {
		return nil
	}
	out, err := t.Command(ctx, "block", "call", "-s", t.Workspace, "-b", t.Pane, "process").Output()
	if err != nil {
		return nil
	}
	var result struct {
		Child *struct {
			PID int `json:"pid"`
		} `json:"child"`
	}
	if json.Unmarshal(out, &result) != nil || result.Child == nil || result.Child.PID < 2 {
		return nil
	}
	return map[int]Target{result.Child.PID: t}
}
