package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type diagnostic struct {
	Check   string `json:"check"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Advice  string `json:"advice,omitempty"`
}

type doctorReport struct {
	Version string       `json:"version"`
	Checks  []diagnostic `json:"checks"`
}

func (r *doctorReport) add(check, status, message, advice string) {
	r.Checks = append(r.Checks, diagnostic{check, status, message, advice})
}

func (r doctorReport) write(w io.Writer, asJSON bool) error {
	if asJSON {
		return printJSON(w, r)
	}
	file, isFile := w.(interface{ Fd() uintptr })
	useColor := isFile && term.IsTerminal(int(file.Fd())) && os.Getenv("NO_COLOR") == "" && colorprofile.Env(os.Environ()) >= colorprofile.ANSI
	return r.writeText(w, useColor)
}

func (r doctorReport) writeText(w io.Writer, useColor bool) error {
	for _, d := range r.Checks {
		label := d.Status + ":"
		if useColor {
			color := ansi.Yellow
			switch d.Status {
			case "ok":
				color = ansi.Green
			case "error":
				color = ansi.Red
			}
			label = ansi.Style{}.ForegroundColor(color).Styled(label)
		}
		if _, err := fmt.Fprintf(w, "%s %s: %s\n", label, d.Check, d.Message); err != nil {
			return err
		}
		if d.Advice != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", d.Advice); err != nil {
				return err
			}
		}
	}
	return nil
}

func newDoctorCommand() *cobra.Command {
	var asJSON bool
	var python string
	cmd := &cobra.Command{
		Use: "doctor", Aliases: []string{"check"},
		Short: "Diagnose integrations, AI review prerequisites and image settings",
		Long: `Check project and global integrations against this build, Python and MCP
requirements for restricted AI reviews, and available multiplexer image settings.
Use --python to check the dedicated Python environment used for Codex reviews.

No AI service is contacted and no settings are changed. Herdr checks read local
configuration, not the running server or a remote client's configuration. Image
support must still be confirmed in the review TUI on the actual terminal.
Errors return exit status 1; optional features and unverified settings produce
warnings or informational results. The former check command remains an alias.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := doctorReport{Version: versionString()}
			r.checkIntegrations(integrations())
			r.checkAI(python)
			r.checkImages()
			if err := r.write(cmd.OutOrStdout(), asJSON); err != nil {
				return err
			}
			n := 0
			for _, d := range r.Checks {
				if d.Status == "error" {
					n++
				}
			}
			if n != 0 {
				return fmt.Errorf("%d diagnostic error(s)", n)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print a structured diagnostic report")
	cmd.Flags().StringVar(&python, "python", "python3", "Python executable used for restricted AI reviews")
	return cmd
}

func (r *doctorReport) checkIntegrations(reg map[string][]integrationFile) {
	names := make([]string, 0, len(reg))
	for name := range reg {
		if name != "prompts" { // Custom prompts need not match bundled templates.
			names = append(names, name)
		}
	}
	sort.Strings(names)
	installed := 0
	checked := map[string]bool{}
	for _, name := range names {
		for _, global := range []bool{false, true} {
			files := reg[name]
			present := false
			for _, f := range files {
				dest, err := integrationDest(f, global)
				if err != nil {
					r.add("integrations", "error", "cannot resolve installation path", "")
					continue
				}
				absolute, err := filepath.Abs(dest)
				if err == nil && checked[absolute] {
					continue
				}
				if _, err := os.Lstat(dest); !os.IsNotExist(err) {
					present = true
				}
			}
			if !present {
				continue
			}
			installed++
			fix := "From the project root, reinstall with `critty install --force " + name + "`."
			if global {
				fix = "From your home directory, reinstall with `critty install --force " + name + "`."
			}
			for _, f := range files {
				dest, err := integrationDest(f, global)
				if err != nil {
					continue
				}
				absolute, err := filepath.Abs(dest)
				if err == nil {
					if checked[absolute] {
						continue
					}
					checked[absolute] = true
				}
				existing, err := os.ReadFile(dest)
				if err != nil {
					r.add("integrations", "error", "missing or unreadable: "+dest, fix)
					continue
				}
				want, err := f.content()
				if err != nil {
					r.add("integrations", "error", "cannot read bundled resource: "+dest, "")
				} else if !bytes.Equal(existing, want) {
					r.add("integrations", "error", "stale: "+dest, fix)
				} else {
					r.add("integrations", "ok", dest, "")
				}
			}
		}
	}
	if installed == 0 {
		r.add("integrations", "info", "no bundled integrations found", "Use `critty install <agent>` to install them.")
	}
}

// All probes are local, have closed stdin and a bounded execution time.
func doctorOutput(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = time.Second
	return commandOutput(cmd)
}

func (r *doctorReport) checkAI(python string) {
	for _, tool := range []string{"claude", "codex"} {
		if path, err := lookPath(tool); err == nil {
			r.add("ai/"+tool, "ok", "available: "+path, "CLI options, authentication and restricted tool access must be verified when launching a review.")
		} else {
			r.add("ai/"+tool, "info", "not on PATH (optional reviewer)", "")
		}
	}
	bin, err := lookPath(python)
	if err != nil {
		r.add("ai/python", "warn", "Python executable not found", "Choose Python 3.11+ with `critty doctor --python /path/to/venv/bin/python` for Codex, or Python 3.10+ for Claude.")
		return
	}
	const probe = `import json, sys
from importlib.metadata import version
sdk = False
mcp = "unavailable"
try:
    mcp = version("mcp")
except Exception:
    pass
try:
    from mcp.server import MCPServer
    from mcp.server.mcpserver.exceptions import ToolError
    sdk = True
except Exception:
    pass
print(json.dumps({"major": sys.version_info.major, "minor": sys.version_info.minor, "mcp": mcp, "sdk": sdk}))`
	out, err := doctorOutput(bin, "-B", "-c", probe)
	var env struct {
		Major int    `json:"major"`
		Minor int    `json:"minor"`
		MCP   string `json:"mcp"`
		SDK   bool   `json:"sdk"`
	}
	if err != nil || json.Unmarshal(out, &env) != nil || env.Major == 0 {
		r.add("ai/python", "warn", "could not inspect the selected Python environment", "Run the selected interpreter directly and check its installation.")
		return
	}
	status := "ok"
	if env.Major < 3 || env.Major == 3 && env.Minor < 10 {
		status = "warn"
	}
	r.add("ai/claude-python", status, fmt.Sprintf("Python %d.%d: %s", env.Major, env.Minor, bin), "Restricted Claude reviews require Python 3.10+.")
	status = "ok"
	if env.Major < 3 || env.Major == 3 && env.Minor < 11 {
		status = "warn"
	}
	r.add("ai/codex-python", status, fmt.Sprintf("Python %d.%d", env.Major, env.Minor), "Restricted Codex reviews require Python 3.11+.")
	requirements, err := skillContent.ReadFile("skill/critty-ai-review/scripts/codex-review-requirements.txt")
	if err != nil {
		r.add("ai/mcp", "error", "cannot read bundled MCP requirements", "")
		return
	}
	expected := strings.TrimPrefix(strings.TrimSpace(string(requirements)), "mcp==")
	status = "ok"
	if !env.SDK || env.MCP != expected {
		status = "warn"
	}
	message := "MCP SDK: " + env.MCP + " (required " + expected + ")"
	if !env.SDK {
		message += "; required server API could not be imported"
	}
	advice := ""
	if status != "ok" {
		advice = mcpInstallAdvice(bin, expected, requirements)
	}
	r.add("ai/mcp", status, message, advice)
}

func mcpInstallAdvice(python, version string, requirements []byte) string {
	reg := integrations()
	for _, name := range agentTargets {
		for _, f := range reg[name] {
			if filepath.Base(f.dest) != "codex-review-requirements.txt" {
				continue
			}
			for _, global := range []bool{false, true} {
				path, err := integrationDest(f, global)
				if err != nil {
					continue
				}
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, requirements) {
					continue
				}
				path, err = filepath.Abs(path)
				if err == nil {
					return "Install into this environment: " + shellEscape(python) + " -m pip install -r " + shellEscape(path)
				}
			}
		}
	}
	return "Install into this environment: " + shellEscape(python) + " -m pip install " + shellEscape("mcp=="+version)
}

func (r *doctorReport) checkImages() {
	if os.Getenv("TMUX") != "" {
		args := []string{"show-options", "-g", "-v", "allow-passthrough"}
		label := "global setting (no pane identity available)"
		if pane := os.Getenv("TMUX_PANE"); pane != "" {
			args = []string{"show-options", "-A", "-p", "-v", "-t", pane, "allow-passthrough"}
			label = "current pane"
		}
		out, err := doctorOutput("tmux", args...)
		value := strings.TrimSpace(string(out))
		status := "warn"
		if err == nil && (value == "on" || value == "all") {
			status = "ok"
		} else if err != nil || value != "off" {
			value = "unknown"
		}
		r.add("images/tmux", status, "allow-passthrough="+value+"; "+label, "For image forwarding, enable `allow-passthrough on` in tmux; pane overrides and every nested multiplexer must also allow graphics.")
	} else {
		r.add("images/tmux", "info", "not in a tmux environment; runtime setting not queried", "Run `critty doctor` in the review terminal to inspect its tmux pane.")
	}
	if _, err := lookPath("herdr"); err == nil || (herdrDetector{}).environmentPresent() {
		r.checkHerdrImages()
	}
	r.add("images/terminal", "info", "outer terminal graphics support is unverified", "Critty requires Kitty graphics with Unicode image placeholders. Open a review on the actual terminal and use Images (`I`) to probe or retry; forwarding settings alone do not prove support.")
}

func (r *doctorReport) checkHerdrImages() {
	path := os.Getenv("HERDR_CONFIG_PATH")
	if path == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			r.add("images/herdr", "warn", "cannot locate local Herdr configuration", "")
			return
		}
		// Herdr uses ~/.config on Unix, including macOS.
		if home, err := os.UserHomeDir(); err == nil && os.PathSeparator == '/' {
			base = filepath.Join(home, ".config")
		}
		path = filepath.Join(base, "herdr", "config.toml")
	}
	var cfg struct {
		Terminal struct {
			KittyGraphics *bool `toml:"kitty_graphics"`
		} `toml:"terminal"`
		Experimental struct {
			KittyGraphics *bool `toml:"kitty_graphics"`
		} `toml:"experimental"`
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data, err = nil, nil
	}
	if err == nil {
		err = toml.Unmarshal(data, &cfg)
	}
	if err != nil {
		r.add("images/herdr", "warn", "local configuration unreadable or invalid: "+path, "Check `herdr config check`; running server and remote client settings are not inspected.")
		return
	}
	enabled := cfg.Terminal.KittyGraphics
	if enabled == nil {
		enabled = cfg.Experimental.KittyGraphics
	}
	status, message := "info", "kitty_graphics unset (current Herdr default: true)"
	if enabled != nil {
		message = fmt.Sprintf("kitty_graphics=%t in %s", *enabled, path)
		if !*enabled {
			status = "warn"
		}
	}
	r.add("images/herdr", status, message, "Set `[terminal] kitty_graphics = true` for images. Restart the server or reattach the client after changing it. This checks local config only; remote server and client settings may differ.")
}

func init() { rootCmd.AddCommand(newDoctorCommand()) }
