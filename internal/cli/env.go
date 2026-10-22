package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/crittylabs/critty/internal/terminalbinding"
	"github.com/spf13/cobra"
)

var envJSON bool

var envCmd = &cobra.Command{
	Use:   "env [--json] [all | tty | run <command> [args...]]",
	Short: "Print the environment that identifies the review terminal, or run a command with it",
	Long: `Locate the tmux, Herdr, or cmux terminal the way review commands do, then print the
variables a process needs to address it: TMUX, TMUX_PANE, and TTY for tmux,
or HERDR_ENV, HERDR_SOCKET_PATH, HERDR_WORKSPACE_ID, HERDR_TAB_ID, HERDR_PANE_ID,
and TTY for Herdr; CMUX_SOCKET_PATH, CMUX_WORKSPACE_ID, and CMUX_SURFACE_ID for cmux.
CRITTY_TERMINAL_APP reports a known application and
CRITTY_SSH=1 reports positive SSH evidence.  By default, or with all, each variable is printed as
NAME=value; --json prints one JSON object instead.  tty prints only the
terminal device.  Flags go before the mode word.  run <command>, or -- <command>, replaces this process with
the command and those variables added to its environment.

Detection uses the multiplexer environment when present and the process
ancestry otherwise, so a tool started from the terminal's process tree can
recover its terminal even without the variables.  Without a pane terminal, TTY
is the controlling terminal of this process or its nearest ancestor, then
SSH_TTY.  A background server such as Codex's daemon has no such ancestry;
pass --terminal-request <id> from a marker prepared by ` + "`critty terminal prepare`" + `
to locate the pane that displays it.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, command, err := parseEnvArgs(cmd.ArgsLenAtDash(), args)
		if err != nil {
			return err
		}
		if envJSON && mode == "run" {
			return fmt.Errorf("--json cannot be combined with a command")
		}
		vars, err := terminalEnvironment()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		switch mode {
		case "run":
			return execWithEnvironment(vars, command)
		case "tty":
			i := slices.IndexFunc(vars, func(v envVar) bool { return v.name == "TTY" })
			if i < 0 {
				return fmt.Errorf("no terminal device found")
			}
			if envJSON {
				return printJSON(out, vars[i].value)
			}
			_, err := fmt.Fprintln(out, vars[i].value)
			return err
		}
		if envJSON {
			object := make(map[string]string, len(vars))
			for _, v := range vars {
				object[v.name] = v.value
			}
			return printJSON(out, object)
		}
		for _, v := range vars {
			if _, err := fmt.Fprintf(out, "%s=%s\n", v.name, v.value); err != nil {
				return err
			}
		}
		return nil
	},
}

// parseEnvArgs accepts a mode word or a command after "--" (dash is its index, -1 when absent).
func parseEnvArgs(dash int, args []string) (mode string, command []string, err error) {
	if dash == 0 {
		if len(args) == 0 {
			return "", nil, fmt.Errorf("run requires a command")
		}
		return "run", args, nil
	}
	if len(args) == 0 {
		return "all", nil, nil
	}
	switch args[0] {
	case "all", "tty":
		if len(args) > 1 {
			return "", nil, fmt.Errorf("%s takes no arguments", args[0])
		}
		return args[0], nil, nil
	case "run":
		if len(args) == 1 {
			return "", nil, fmt.Errorf("run requires a command")
		}
		return "run", args[1:], nil
	}
	return "", nil, fmt.Errorf("unknown mode %q; use all, tty, run <command>, or -- <command>", args[0])
}

type envVar struct {
	name  string
	value string
}

func init() {
	envCmd.Flags().BoolVar(&envJSON, "json", false, "output as JSON")
	envCmd.Flags().SetInterspersed(false)
	rootCmd.AddCommand(envCmd)
}

func terminalEnvironment() ([]envVar, error) {
	ctx, err := reviewTerminalContext()
	if err != nil {
		return nil, err
	}
	vars, err := environmentForTerminal(ctx)
	if err == nil && len(vars) == 0 {
		return nil, fmt.Errorf("no review terminal or client hints found")
	}
	return vars, err
}

func environmentForTerminal(ctx reviewMultiplexerContext) ([]envVar, error) {
	var err error
	var vars []envVar
	switch ctx := ctx.(type) {
	case cmuxContext:
		vars = []envVar{{"CMUX_SOCKET_PATH", ctx.socket}, {"CMUX_WORKSPACE_ID", ctx.workspace}, {"CMUX_SURFACE_ID", ctx.surface}}
	case tmuxContext:
		vars, err = tmuxEnvironment(ctx)
	case herdrContext:
		vars, err = herdrEnvironment(ctx)
	}
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(vars, func(v envVar) bool { return v.name == "TTY" }) {
		if tty := fallbackTTY(); tty != "" {
			vars = append(vars, envVar{"TTY", tty})
		}
	}
	origin := originForReview()
	if origin.Terminal != "" {
		vars = append(vars, envVar{"CRITTY_TERMINAL_APP", origin.Terminal})
	}
	if terminalbinding.Remote(terminalTarget(ctx), origin) {
		vars = append(vars, envVar{"CRITTY_SSH", "1"})
	}
	return vars, nil
}

// fallbackTTY reports the controlling terminal of this process or its nearest
// ancestor, then SSH_TTY.  Hooks often run detached from the terminal that
// started their agent.
func fallbackTTY() string {
	if tty := processTTY(os.Getpid()); tty != "" {
		return tty
	}
	seen := make(map[int]bool)
	for pid := parentProcessID(); pid > 1 && !seen[pid]; {
		seen[pid] = true
		if tty := processTTY(pid); tty != "" {
			return tty
		}
		ppid, err := inspectProcess(pid)
		if err != nil {
			break
		}
		pid = ppid
	}
	return os.Getenv("SSH_TTY")
}

// processTTY returns the controlling terminal device of a process, or "" when it has none.
func processTTY(pid int) string {
	out, err := commandOutput(exec.Command("ps", "-o", "tty=", "-p", strconv.Itoa(pid)))
	if err != nil {
		return ""
	}
	tty := strings.TrimSpace(string(out))
	if tty == "" || tty == "-" || strings.HasPrefix(tty, "?") {
		return ""
	}
	if !strings.HasPrefix(tty, "/") {
		tty = "/dev/" + tty
	}
	return tty
}

// tmuxEnvironment asks the server for the pane's current identity, so a
// context known only by socket and pane also yields TMUX.
func tmuxEnvironment(tmux tmuxContext) ([]envVar, error) {
	if tmux.pane == "" {
		if tmux.session == "" {
			return nil, fmt.Errorf("tmux pane unknown")
		}
		return []envVar{{"TMUX", tmux.session}}, nil
	}
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return nil, err
	}
	out, err := commandOutput(tmuxCommand(tmuxBin, tmux, "display-message", "-p", "-t", tmux.pane, "#{socket_path}\t#{pid}\t#{session_id}\t#{pane_id}\t#{pane_tty}"))
	if err != nil {
		return nil, fmt.Errorf("querying tmux pane %s: %w", tmux.pane, err)
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\t")
	if len(fields) != 5 || slices.Contains(fields[:4], "") {
		return nil, fmt.Errorf("unexpected tmux pane description: %q", out)
	}
	vars := []envVar{
		{"TMUX", strings.Join([]string{fields[0], fields[1], strings.TrimPrefix(fields[2], "$")}, ",")},
		{"TMUX_PANE", fields[3]},
	}
	if fields[4] != "" {
		vars = append(vars, envVar{"TTY", fields[4]})
	}
	return vars, nil
}

func herdrEnvironment(herdr herdrContext) ([]envVar, error) {
	socket := herdr.socket
	if socket == "" {
		socket = os.Getenv("HERDR_SOCKET_PATH")
	}
	vars := []envVar{{"HERDR_ENV", "1"}}
	if socket != "" {
		vars = append(vars, envVar{"HERDR_SOCKET_PATH", socket})
	}
	vars = append(vars,
		envVar{"HERDR_WORKSPACE_ID", herdr.workspace},
		envVar{"HERDR_TAB_ID", herdr.tab},
		envVar{"HERDR_PANE_ID", herdr.pane},
	)
	if tty := herdrPaneTTY(herdr); tty != "" {
		vars = append(vars, envVar{"TTY", tty})
	}
	return vars, nil
}

// herdrPaneTTY is best effort: a pane without a shell has no terminal to report.
func herdrPaneTTY(herdr herdrContext) string {
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return ""
	}
	out, err := commandOutput(herdr.command(herdrBin, "pane", "process-info", "--pane", herdr.pane))
	if err != nil {
		return ""
	}
	var response struct {
		Result struct {
			ProcessInfo struct {
				TTY string `json:"tty"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &response) != nil {
		return ""
	}
	tty := response.Result.ProcessInfo.TTY
	if tty != "" && !strings.HasPrefix(tty, "/") {
		tty = "/dev/" + tty
	}
	return tty
}

func execWithEnvironment(vars []envVar, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, mergeEnvironment(os.Environ(), vars))
}

// mergeEnvironment replaces existing values of the given variables and appends the rest.
func mergeEnvironment(environ []string, vars []envVar) []string {
	merged := make([]string, 0, len(environ)+len(vars))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.ContainsFunc(vars, func(v envVar) bool { return v.name == name }) {
			merged = append(merged, entry)
		}
	}
	for _, v := range vars {
		merged = append(merged, v.name+"="+v.value)
	}
	return merged
}
