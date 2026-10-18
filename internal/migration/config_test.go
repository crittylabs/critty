package migration

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/crittylabs/critty/internal/config"
)

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigConversionPreservesValuesAndRereadsTOML(t *testing.T) {
	_, root, project := setup(t)
	global := filepath.Join(root, "critty", "config.json")
	local := filepath.Join(project, ".critty.config.json")
	writeText(t, global, `{
		"author": "Global", "quiet": true, "cleanup_on_approve": false,
		"launchers": ["mux"], "ignore_patterns": ["*.lock"],
		"prompts": {"on_finish_approved:diff": "inline:line one\nline two\\tail"},
		"future": {"count": 9007199254740993, "ratio": 1.25, "enabled": false}
	}`)
	writeText(t, local, `{"author":"Project 日本語","quiet":false,"launchers":[],"ignore_patterns":["*.lock","*.min.js"]}`)
	if err := os.Chmod(global, 0640); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run(project, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "critty: migrated ") != 2 {
		t.Fatal(output.String())
	}
	for _, path := range []string{global, local} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("JSON source remains: %s: %v", path, err)
		}
	}
	info, err := os.Stat(config.GlobalPath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(config.GlobalPath())
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Future struct {
			Count   int64
			Ratio   float64
			Enabled bool
		}
	}
	if err := toml.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Future.Count != 9007199254740993 || saved.Future.Ratio != 1.25 || saved.Future.Enabled {
		t.Fatalf("unknown values changed: %+v", saved)
	}
	cfg, err := config.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Author != "Project 日本語" || cfg.Quiet || cfg.CleanupOnApprove || cfg.Launchers == nil || len(cfg.Launchers) != 0 {
		t.Fatalf("merged configuration changed: %+v", cfg)
	}
	if strings.Join(cfg.IgnorePatterns, ",") != "*.lock,*.min.js" || cfg.Prompts["on_finish_approved:diff"] != "inline:line one\nline two\\tail" {
		t.Fatalf("patterns or prompt changed: %+v", cfg)
	}
	output.Reset()
	if err := Run(project, &output); err != nil || output.Len() != 0 {
		t.Fatalf("migration repeated: %v: %s", err, output.String())
	}
}

func TestConfigConversionFailureKeepsJSON(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"malformed", `{"author":`},
		{"trailing value", `{} {}`},
		{"not object", `[]`},
		{"null object", `null`},
		{"null value", `{"future":{"value":null}}`},
		{"integer overflow", `{"future":9223372036854775808}`},
		{"float overflow", `{"future":1e999}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, root, project := setup(t)
			path := filepath.Join(root, "critty", "config.json")
			writeText(t, path, tc.content)
			var output bytes.Buffer
			if err := Run(project, &output); err == nil || output.Len() != 0 {
				t.Fatalf("failed migration reported success: %v: %s", err, output.String())
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.content {
				t.Fatalf("JSON changed: %v: %s", err, data)
			}
			if _, err := os.Lstat(config.GlobalPath()); !os.IsNotExist(err) {
				t.Fatalf("failed migration published TOML: %v", err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 || entries[0].Name() != "config.json" {
				t.Fatalf("failed migration left staging files: %v: %v", entries, err)
			}
		})
	}
}

func TestInvalidTOMLDoesNotFallBackToJSON(t *testing.T) {
	_, root, project := setup(t)
	writeText(t, config.GlobalPath(), "author = [")
	legacy := filepath.Join(root, "critty", "config.json")
	writeText(t, legacy, `{"author":"Do not load"}`)
	var output bytes.Buffer
	if err := Run(project, &output); err != nil || output.Len() != 0 {
		t.Fatalf("existing TOML triggered migration: %v: %s", err, output.String())
	}
	if _, err := config.Load(project); err == nil {
		t.Fatal("invalid TOML silently fell back")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal(err)
	}
}
