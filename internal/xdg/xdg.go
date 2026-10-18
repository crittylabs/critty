// Package xdg resolves Critty's state and config directories following the
// XDG Base Directory specification on every platform, so review data lives
// in predictable, user-relocatable locations.
package xdg

import (
	"os"
	"path/filepath"
)

// StateHome returns the base state directory ($XDG_STATE_HOME or
// ~/.local/state) with the critty application directory appended.
func StateHome() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "state")
	}
	return filepath.Join(base, "critty")
}

// ConfigHome returns the base config directory ($XDG_CONFIG_HOME or
// ~/.config) with the critty application directory appended.
func ConfigHome() string {
	return AppConfigHome("critty")
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
