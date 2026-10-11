// Package xdg resolves Critty's state and config directories following the
// XDG Base Directory specification on every platform, so review data lives
// in predictable, user-relocatable locations.
package xdg

import (
	"os"
	"path/filepath"
)

const appDir = "tcrit"

// StateHome returns the base state directory ($XDG_STATE_HOME or
// ~/.local/state) with the tcrit application directory appended.
// Review sessions, plans, and other mutable state live here.
func StateHome() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, appDir)
	}
	return filepath.Join(homeDir(), ".local", "state", appDir)
}

// ConfigHome returns the base config directory ($XDG_CONFIG_HOME or
// ~/.config) with the tcrit application directory appended.
func ConfigHome() string {
	return AppConfigHome(appDir)
}

// AppConfigHome returns the XDG config directory of another application,
// such as the global OpenCode config directory.
func AppConfigHome(app string) string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, app)
	}
	return filepath.Join(homeDir(), ".config", app)
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
