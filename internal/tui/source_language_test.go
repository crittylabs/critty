package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/crittylabs/critty/internal/document"
)

func TestSourceLanguageDetection(t *testing.T) {
	for _, tc := range []struct {
		name, path, source, want string
	}{
		{"python", "tool", "#!/usr/bin/python3.14 -u\n", "python"},
		{"env", "tool", "#!/usr/bin/env ruby\n", "ruby"},
		{"env split", "tool", "#!/usr/bin/env -S python3 -u\n", "python"},
		{"env long split", "tool", "#!/usr/bin/env --split-string='node --no-warnings'\n", "javascript"},
		{"env options", "tool", "#!/usr/bin/env -i -u OLD --chdir=/tmp MODE=test perl\n", "perl"},
		{"env terminator", "tool", "#!/usr/bin/env -- lua5.4\r\n", "lua"},
		{"shell", "tool", "#!/bin/dash\n", "bash"},
		{"typescript", "tool", "#!/usr/bin/env ts-node\n", "typescript"},
		{"text suffix", "tool.txt", "#!/usr/bin/env python3\n", "python"},
		{"known suffix", "tool.rb", "#!/usr/bin/env python3\n", "ruby"},
		{"C++ header", "service.h", "", "cpp"},
		{"gemspec", "example.gemspec", "", "ruby"},
		{"Gemfile", "config/Gemfile", "", "ruby"},
		{"Gemfile variant", "Gemfile.ci", "", "ruby"},
		{"Rakefile variant", "Rakefile.local", "", "ruby"},
		{"Podfile", "Podfile", "", "ruby"},
		{"Vagrantfile", "Vagrantfile", "", "ruby"},
		{"bashrc", ".bashrc", "", "bash"},
		{"zshrc", ".zshrc", "", "bash"},
		{"bash profile", ".bash_profile", "", "bash"},
		{"zprofile", ".zprofile", "", "bash"},
		{"profile", ".profile", "", "bash"},
		{"Emacs init", ".emacs", "", "emacs-lisp"},
		{"Vim init", ".vimrc", "", "vim"},
		{"plain file", "notes", "class Service:\n", ""},
		{"Gemfile lock", "Gemfile.lock", "GEM\n  specs:\n", ""},
		{"missing interpreter", "tool", "#!/usr/bin/env -S\n", ""},
		{"incomplete option", "tool", "#!/usr/bin/env --unset\n", ""},
		{"unknown interpreter", "tool", "#!/bin/unknown\n", ""},
		{"broken quoting", "tool", "#!/usr/bin/env 'python3\n", ""},
		{"later line", "tool", "text\n#!/usr/bin/env python3\n", ""},
		{"oversized header", "tool", "#!/usr/bin/env " + strings.Repeat(" ", 4096) + "python3\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := codeOutlineLanguage(tc.path, tc.source); got != tc.want {
				t.Fatalf("language = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectedScriptUsesOneLanguageForDisplayAndOutline(t *testing.T) {
	source := "#!/usr/bin/env -S python3 -u\nclass Service:\n  def run(self):\n    return 1\n"
	tab := FileTab{path: "tool", doc: document.FromContent("tool", []byte(source))}
	tab.ensureHighlightCache()
	if !tab.isCode || tab.isMarkdown {
		t.Fatal("extensionless script was treated as a text document")
	}
	if len(tab.chromaLines) != 4 || ansi.Strip(tab.chromaLines[1]) != "class Service:" || tab.chromaLines[1] == "class Service:" {
		t.Fatalf("script was not highlighted without changing its text: %q", tab.chromaLines)
	}
	outline := tab.ensureCodeOutline()
	if outline == nil || len(outline.rows) != 2 || outline.rows[1] != (outlineHeading{3, 2, "def run"}) {
		t.Fatalf("script outline does not match its display language: %+v", outline)
	}
}
