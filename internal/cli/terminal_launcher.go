package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type terminalCLI struct {
	binary    string
	bundles   []string
	windows   bool
	linuxOnly bool
}

var terminalCLIs = map[string]terminalCLI{
	"rio-window":       {binary: "rio", bundles: []string{"Rio.app", "rio.app"}, windows: true},
	"wezterm-window":   {binary: "wezterm", bundles: []string{"WezTerm.app"}, windows: true},
	"alacritty-window": {binary: "alacritty", bundles: []string{"Alacritty.app"}, windows: true},
	"kitty-window":     {binary: "kitty", bundles: []string{"kitty.app"}},
	"ghostty-window":   {binary: "ghostty", linuxOnly: true},
	"xterm-window":     {binary: "xterm", linuxOnly: true},
}

func (l *desktopLaunch) usesCLI() bool {
	_, ok := terminalCLIs[l.name]
	return ok && (l.name != "ghostty-window" || runtime.GOOS != "darwin")
}

func terminalExecutable(name, platform string, wsl bool) (string, error) {
	spec, ok := terminalCLIs[name]
	if !ok || platform != "darwin" && platform != "linux" || spec.linuxOnly && platform != "linux" {
		return "", fmt.Errorf("launcher %s is unavailable on %s", name, platform)
	}
	if wsl {
		if !spec.windows {
			return "", fmt.Errorf("launcher %s does not support Windows terminals under WSL", name)
		}
		return lookPath(spec.binary + ".exe")
	}
	bin, err := lookPath(spec.binary)
	if err == nil || platform != "darwin" {
		return bin, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
		for _, app := range spec.bundles {
			bin := filepath.Join(dir, app, "Contents", "MacOS", spec.binary)
			if info, err := os.Stat(bin); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
				return bin, nil
			}
		}
	}
	return "", fmt.Errorf("%s CLI not found on PATH or in an Applications directory", spec.binary)
}

func (l *desktopLaunch) terminalCommand(platform, command, cwd string) (*exec.Cmd, error) {
	bin, err := terminalExecutable(l.name, platform, l.origin.WSLDistribution != "")
	if err != nil {
		return nil, err
	}
	var args []string
	switch l.name {
	case "rio-window":
		args = []string{"--title-placeholder", "Critty"}
		if l.origin.WSLDistribution == "" {
			args = append(args, "--working-dir", cwd)
		}
		args = append(args, "-e")
	case "wezterm-window":
		args = []string{"start", "--always-new-process", "--no-auto-connect", "--domain", "local"}
		if l.origin.WSLDistribution == "" {
			args = append(args, "--cwd", cwd)
		}
		args = append(args, "--")
	case "alacritty-window":
		args = []string{"--title", "Critty"}
		if l.origin.WSLDistribution == "" {
			args = append(args, "--working-directory", cwd)
		}
		args = append(args, "-e")
	case "kitty-window":
		args = []string{"--title", "Critty", "--directory", cwd, "--session", "none"}
	case "ghostty-window":
		args = []string{"--title=Critty", "--working-directory=" + cwd, "-e"}
	case "xterm-window":
		args = []string{"-T", "Critty", "-e"}
	}
	if l.origin.WSLDistribution != "" {
		payload := base64.StdEncoding.EncodeToString([]byte(command))
		args = append(args, "wsl.exe", "--distribution", l.origin.WSLDistribution,
			"--exec", "/bin/sh", "-c", `exec /bin/sh -c "$(printf %s "$1" | base64 -d)"`, "critty", payload)
	} else {
		args = append(args, "/bin/sh", "-c", command)
	}
	return exec.Command(bin, args...), nil
}
