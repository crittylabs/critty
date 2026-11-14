// Package syntax resolves user-defined language aliases and filename rules.
package syntax

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

type Config struct {
	Aliases map[string]string `toml:"aliases" json:"aliases,omitempty"`
	Files   []FileType        `toml:"files" json:"files,omitempty"`
}

type FileType struct {
	Pattern  string `toml:"pattern" json:"pattern"`
	Language string `toml:"language" json:"language"`
}

// Merge gives project aliases and filename rules precedence over global ones.
// Rules within each level keep their declaration order; the first match wins.
func Merge(base, project Config) Config {
	result := Config{Aliases: make(map[string]string), Files: slices.Concat(project.Files, base.Files)}
	for _, cfg := range []Config{base, project} {
		for name, language := range cfg.Aliases {
			result.Aliases[normalize(name)] = strings.TrimSpace(language)
		}
	}
	return result
}

// ValidateNames catches ambiguous case variants before configuration levels merge.
func (c Config) ValidateNames() error {
	seen := make(map[string]bool)
	for _, name := range slices.Sorted(maps.Keys(c.Aliases)) {
		key := normalize(name)
		if key == "" || len(strings.Fields(key)) != 1 {
			return fmt.Errorf("syntax.aliases: invalid name %q", name)
		}
		if seen[key] {
			return fmt.Errorf("syntax.aliases: duplicate name %q (case-insensitive)", name)
		}
		seen[key] = true
	}
	return nil
}

type fileRule struct {
	pattern string
	lexer   chroma.Lexer
}

// Rules is immutable after compilation and can be shared by concurrent renderers.
// A nil Rules value retains built-in language detection.
type Rules struct {
	aliases map[string]chroma.Lexer
	files   []fileRule
}

func Compile(cfg Config) (*Rules, error) {
	if err := cfg.ValidateNames(); err != nil {
		return nil, err
	}
	if len(cfg.Aliases) == 0 && len(cfg.Files) == 0 {
		return nil, nil
	}
	names := make(map[string]string, len(cfg.Aliases))
	for name, language := range cfg.Aliases {
		names[normalize(name)] = strings.TrimSpace(language)
	}
	rules := &Rules{aliases: make(map[string]chroma.Lexer, len(names))}
	visiting := make(map[string]bool)
	var resolve func(string) (chroma.Lexer, error)
	resolve = func(name string) (chroma.Lexer, error) {
		key := normalize(name)
		if lexer := rules.aliases[key]; lexer != nil {
			return lexer, nil
		}
		if target, ok := names[key]; ok {
			if visiting[key] {
				return nil, fmt.Errorf("syntax.aliases: cycle involving %q", key)
			}
			visiting[key] = true
			lexer, err := resolve(target)
			if err != nil {
				return nil, fmt.Errorf("alias %q: %w", key, err)
			}
			visiting[key] = false
			rules.aliases[key] = lexer
			return lexer, nil
		}
		if lexer := lexers.Get(strings.TrimSpace(name)); lexer != nil {
			return lexer, nil
		}
		return nil, fmt.Errorf("unknown syntax language %q", name)
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		if _, err := resolve(name); err != nil {
			return nil, err
		}
	}
	for _, file := range cfg.Files {
		if file.Pattern == "" || strings.Contains(file.Pattern, "/") {
			return nil, fmt.Errorf("syntax.files pattern %q must match a filename without directories", file.Pattern)
		}
		if _, err := path.Match(file.Pattern, ""); err != nil {
			return nil, fmt.Errorf("syntax.files pattern %q: %w", file.Pattern, err)
		}
		lexer, err := resolve(file.Language)
		if err != nil {
			return nil, fmt.Errorf("syntax.files pattern %q: %w", file.Pattern, err)
		}
		rules.files = append(rules.files, fileRule{file.Pattern, lexer})
	}
	return rules, nil
}

func (r *Rules) HasAlias(name string) bool {
	return r != nil && r.aliases[normalize(name)] != nil
}

// Lexer resolves a fence language name, including overrides of built-in aliases.
func (r *Rules) Lexer(name string) chroma.Lexer {
	if r != nil {
		if lexer := r.aliases[normalize(name)]; lexer != nil {
			return lexer
		}
	}
	return lexers.Get(name)
}

// Match checks case-sensitive basename globs before the built-in filename rules.
func (r *Rules) Match(filename string) (chroma.Lexer, bool) {
	if r != nil {
		name := filepath.Base(filename)
		for _, rule := range r.files {
			if matched, _ := path.Match(rule.pattern, name); matched {
				return rule.lexer, true
			}
		}
	}
	return nil, false
}

func normalize(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
