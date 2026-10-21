package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Textconv identifies the representation whose lines are used in a review.
// Command is Git configuration, not a command supplied by review JSON.
type Textconv struct {
	Driver  string `json:"driver"`
	Command string `json:"command"`
}

func textconvForPath(path string, enabled []string) (Textconv, error) {
	conversions, err := textconvsForPaths([]string{path}, enabled)
	return conversions[path], err
}

func textconvsForPaths(paths, patterns []string) (map[string]Textconv, error) {
	conversions := make(map[string]Textconv, len(paths))
	if len(paths) == 0 || len(patterns) == 0 {
		return conversions, nil
	}
	cmd := exec.Command("git", "check-attr", "-z", "--stdin", "diff")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading diff attributes: %w", err)
	}
	fields := strings.Split(string(out), "\x00")
	if len(fields) != 3*len(paths)+1 {
		return nil, fmt.Errorf("invalid diff attributes")
	}
	drivers := map[string]Textconv{}
	for i := 0; i < len(fields)-1; i += 3 {
		path, driver := fields[i], fields[i+2]
		enabled, err := textconvEnabled(driver, patterns)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		conversion, ok := drivers[driver]
		if !ok {
			conversion, err = textconvDriver(driver)
			if err != nil {
				return nil, err
			}
			drivers[driver] = conversion
		}
		conversions[path] = conversion
	}
	return conversions, nil
}

func textconvEnabled(driver string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		if pattern == "*" {
			return true, nil
		}
		matched, err := path.Match(pattern, driver)
		if err != nil {
			return false, fmt.Errorf("invalid textconv_drivers pattern %q: %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func textconvDriver(driver string) (Textconv, error) {
	switch driver {
	case "set", "unset", "unspecified":
		return Textconv{}, nil
	}
	cmd := exec.Command("git", "config", "--get", "diff."+driver+".textconv")
	data, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return Textconv{}, nil
		}
		return Textconv{}, fmt.Errorf("reading textconv driver %s: %w", driver, err)
	}
	command := strings.TrimSuffix(string(data), "\n")
	if command == "" {
		return Textconv{}, nil
	}
	return Textconv{Driver: driver, Command: command}, nil
}

// TextContent applies the configured driver to the selected revision.  It never
// executes metadata read from a review; callers obtain the driver from Git.
func (s ReviewSource) TextContent(path string, conversion Textconv) ([]byte, error) {
	if conversion.Command == "" {
		return s.Content(path)
	}
	enabled, err := textconvEnabled(conversion.Driver, s.TextconvDrivers)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return s.Content(path)
	}
	var data []byte
	switch s.Scope {
	case "staged", "range":
		ref := ":" + path
		if s.Scope == "range" {
			ref = s.Head + ":" + path
		}
		var out string
		out, err = gitCommand("cat-file", "--textconv", ref)
		data = []byte(out)
	default:
		// Git appends the filename as one shell argument.  Keep it separate
		// from the configured command, including spaces and shell metacharacters.
		var abs string
		abs, err = filepath.Abs(path)
		if err == nil {
			cmd := exec.Command("sh", "-c", conversion.Command+` "$@"`, "textconv", abs)
			data, err = cmd.Output()
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("textconv %s for %s: %w", conversion.Driver, path, err)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("textconv %s for %s did not produce UTF-8 text", conversion.Driver, path)
	}
	return data, nil
}
