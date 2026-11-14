package syntax

import (
	"strings"
	"testing"
)

func TestRules(t *testing.T) {
	rules, err := Compile(Config{
		Aliases: map[string]string{"custom": "go", "chain": "CUSTOM", "js": "json"},
		Files:   []FileType{{"special.demo", "python"}, {"*.demo", "chain"}, {"*.md", "text"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, want string }{{"custom", "Go"}, {"CHAIN", "Go"}, {"js", "JSON"}, {"ruby", "Ruby"}} {
		if got := rules.Lexer(tc.name); got == nil || got.Config().Name != tc.want {
			t.Errorf("Lexer(%q) = %v, want %s", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct{ path, want string }{
		{"dir/special.demo", "Python"}, {"dir/other.demo", "Go"}, {"dir/SPECIAL.demo", "Go"}, {"notes.md", "plaintext"},
	} {
		lexer, ok := rules.Match(tc.path)
		if !ok || lexer.Config().Name != tc.want {
			t.Errorf("Match(%q) = %v, %t, want %s", tc.path, lexer, ok, tc.want)
		}
	}
	if _, ok := rules.Match("unmatched.go"); ok {
		t.Fatal("unmatched name should fall through to built-in detection")
	}
	if !rules.HasAlias("CUSTOM") || rules.HasAlias("ruby") {
		t.Fatal("custom and built-in aliases are not distinguished")
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want string
	}{
		{"unknown alias target", Config{Aliases: map[string]string{"custom": "no-such-language"}}, "unknown syntax language"},
		{"empty alias target", Config{Aliases: map[string]string{"custom": ""}}, "unknown syntax language"},
		{"cycle", Config{Aliases: map[string]string{"one": "two", "two": "one"}}, "cycle"},
		{"duplicate case", Config{Aliases: map[string]string{"custom": "go", "CUSTOM": "ruby"}}, "duplicate name"},
		{"empty alias", Config{Aliases: map[string]string{"": "go"}}, "invalid name"},
		{"invalid pattern", Config{Files: []FileType{{"[", "go"}}}, "syntax.files pattern"},
		{"directory pattern", Config{Files: []FileType{{"src/*.go", "go"}}}, "without directories"},
		{"empty pattern", Config{Files: []FileType{{"", "go"}}}, "without directories"},
		{"unknown file language", Config{Files: []FileType{{"*.demo", "no-such-language"}}}, "unknown syntax language"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Compile() error = %v, want %s", err, tc.want)
			}
		})
	}
}
