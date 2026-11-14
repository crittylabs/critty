package config

import (
	"strings"
	"testing"
)

func TestSyntaxConfigErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"unknown", "[syntax.aliases]\ncustom = 'nonexistent-format'", "unknown syntax language"},
		{"cycle", "[syntax.aliases]\na = 'b'\nb = 'a'", "cycle"},
		{"pattern", "[[syntax.files]]\npattern = '['\nlanguage = 'go'", "syntax.files pattern"},
		{"duplicate", "[syntax.aliases]\ncustom = 'go'\nCUSTOM = 'ruby'", "duplicate name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			project := t.TempDir()
			writeConfig(t, ProjectPath(project), tc.body)
			if _, err := Load(project); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSyntaxConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	writeConfig(t, GlobalPath(), `
[syntax.aliases]
custom = "go"
chained = "custom"
inherited = "python"
[[syntax.files]]
pattern = "*.demo"
language = "custom"
[[syntax.files]]
pattern = "*.note"
language = "markdown"
`)
	writeConfig(t, ProjectPath(project), `
[syntax.aliases]
CUSTOM = "ruby"
[[syntax.files]]
pattern = "*.demo"
language = "json"
`)
	cfg, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ alias, want string }{{"custom", "Ruby"}, {"chained", "Ruby"}, {"inherited", "Python"}} {
		if got := cfg.SyntaxRules.Lexer(tc.alias); got == nil || got.Config().Name != tc.want {
			t.Fatalf("alias %s = %v, want %s", tc.alias, got, tc.want)
		}
	}
	for _, tc := range []struct{ file, want string }{{"test.demo", "JSON"}, {"test.note", "markdown"}} {
		if got, ok := cfg.SyntaxRules.Match(tc.file); !ok || got.Config().Name != tc.want {
			t.Fatalf("file %s = %v, want %s", tc.file, got, tc.want)
		}
	}
}
