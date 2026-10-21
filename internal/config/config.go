// Package config loads Critty configuration from the XDG config directory
// (global) and the project root (.critty.toml), mirroring crit's
// two-level merge semantics for the subset of keys Critty supports.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/crittylabs/critty/internal/history"
	"github.com/crittylabs/critty/internal/xdg"
)

// Config is the resolved (merged) configuration.
type Config struct {
	History          history.Policy    `json:"history"`
	Output           string            `json:"output,omitempty"`
	Author           string            `json:"author,omitempty"`
	Split            bool              `json:"split,omitempty"`
	IgnorePatterns   []string          `json:"ignore_patterns,omitempty"`
	Launchers        []string          `json:"launchers,omitempty"`
	TextconvDrivers  []string          `json:"textconv_drivers,omitempty"`
	Quiet            bool              `json:"quiet,omitempty"`
	CleanupOnApprove bool              `json:"cleanup_on_approve,omitempty"`
	DisableStats     bool              `json:"disable_stats,omitempty"`
	Prompts          map[string]string `json:"prompts,omitempty"`

	// ProjectRoot is the directory the project-level config was resolved
	// against (git top level or cwd).  Not part of the file format.
	ProjectRoot string `json:"-"`
}

// fileConfig is the on-disk shape; booleans are pointers so a project file
// can explicitly override a global true with false.
type fileConfig struct {
	History          *history.Policy   `toml:"history"`
	Output           string            `toml:"output"`
	Author           string            `toml:"author"`
	Split            *bool             `toml:"split"`
	IgnorePatterns   []string          `toml:"ignore_patterns"`
	Launchers        []string          `toml:"launchers"`
	TextconvDrivers  []string          `toml:"textconv_drivers"`
	Quiet            *bool             `toml:"quiet"`
	CleanupOnApprove *bool             `toml:"cleanup_on_approve"`
	DisableStats     *bool             `toml:"disable_stats"`
	Prompts          map[string]string `toml:"prompts"`
}

// GlobalPath returns the global config file path.
func GlobalPath() string {
	return filepath.Join(xdg.ConfigHome(), "config.toml")
}

// ProjectPath returns the project config file path under root.
func ProjectPath(root string) string {
	return filepath.Join(root, ".critty.toml")
}

// Load merges global and project configuration.  projectRoot may be empty
// to skip the project level.  Missing files are not errors.
func Load(projectRoot string) (*Config, error) {
	cfg := &Config{}

	global, err := readFile(GlobalPath())
	if err != nil {
		return nil, err
	}
	apply(cfg, global)
	// Retention can delete saved data, so repositories cannot enable it.
	if global != nil && global.History != nil {
		cfg.History = *global.History
	}

	if projectRoot != "" {
		project, err := readFile(ProjectPath(projectRoot))
		if err != nil {
			return nil, err
		}
		apply(cfg, project)
	}

	for _, pattern := range cfg.TextconvDrivers {
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid textconv_drivers pattern %q: %w", pattern, err)
		}
	}
	cfg.History, err = cfg.History.Normalize()
	if err != nil {
		return nil, err
	}
	if cfg.Author == "" {
		cfg.Author = gitUserName()
	}

	return cfg, nil
}

// LoadCurrent loads configuration for the current directory, using the git
// top-level directory as the project root when inside a repository.
func LoadCurrent() (*Config, error) {
	root := ""
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	} else if cwd, err := os.Getwd(); err == nil {
		root = cwd
	}
	cfg, err := Load(root)
	if err != nil {
		return nil, err
	}
	cfg.ProjectRoot = root
	return cfg, nil
}

func readFile(path string) (*fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var fc fileConfig
	if err := toml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &fc, nil
}

func apply(cfg *Config, fc *fileConfig) {
	if fc == nil {
		return
	}
	if fc.Output != "" {
		cfg.Output = fc.Output
	}
	if fc.Author != "" {
		cfg.Author = fc.Author
	}
	if fc.Split != nil {
		cfg.Split = *fc.Split
	}
	// Pattern lists are unioned across levels, like crit.
	cfg.IgnorePatterns = appendUnique(cfg.IgnorePatterns, fc.IgnorePatterns)
	if fc.Launchers != nil {
		cfg.Launchers = slices.Clone(fc.Launchers)
	}
	if fc.TextconvDrivers != nil {
		cfg.TextconvDrivers = slices.Clone(fc.TextconvDrivers)
	}
	if fc.Quiet != nil {
		cfg.Quiet = *fc.Quiet
	}
	if fc.CleanupOnApprove != nil {
		cfg.CleanupOnApprove = *fc.CleanupOnApprove
	}
	if fc.DisableStats != nil {
		cfg.DisableStats = *fc.DisableStats
	}
	for k, v := range fc.Prompts {
		if cfg.Prompts == nil {
			cfg.Prompts = map[string]string{}
		}
		cfg.Prompts[k] = v
	}
}

func appendUnique(dst, src []string) []string {
	for _, s := range src {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}

func gitUserName() string {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
