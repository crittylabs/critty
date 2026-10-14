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
// Existing data stays in place, with a new alias so running sessions and
// stored absolute paths keep working.  If aliasing is unavailable or the
// new path holds independent data, keep using the legacy directory.
func StateHome() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "state")
	}
	current, legacy := filepath.Join(base, "critty"), filepath.Join(base, "tcrit")
	old, err := os.Stat(legacy)
	if err != nil || !old.IsDir() {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(current, 0700); err == nil {
				// Older launchers must see the same registry on fresh installs.
				if err := os.Symlink("critty", legacy); err != nil {
					if info, err := os.Stat(legacy); err == nil && info.IsDir() {
						return legacy
					}
				}
			}
		}
		return current
	}
	// A relative target also works when XDG_STATE_HOME is relative.
	if err := os.Symlink("tcrit", current); err == nil {
		return current
	}
	if info, err := os.Stat(current); err == nil && os.SameFile(old, info) {
		return current
	}
	return legacy
}

// ConfigHome returns the base config directory ($XDG_CONFIG_HOME or
// ~/.config) with the critty application directory appended.
func ConfigHome() string {
	return AppConfigHome("critty")
}

// LegacyConfigHome returns the former TCrit configuration directory.
func LegacyConfigHome() string {
	return AppConfigHome("tcrit")
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
