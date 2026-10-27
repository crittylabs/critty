package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func newHelpTestCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "critty [file]",
		Version: "v1.2.3",
		Short:   "Review changes",
		Long:    "Review changes from the terminal.  Run `critty --staged` to review only the index, or `critty <file>` to review a document with a rather long description that needs wrapping.",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	root.Flags().String("scope", "", "review scope: all (default), staged, unstaged, A..B, or A...B (omitted B means HEAD)")
	root.Flags().StringP("output", "o", "", "review data root")
	root.PersistentFlags().StringArray("launcher", nil, "review launcher: mux, tmux-floating, tmux-split, desktop-terminal, or custom:EXECUTABLE")
	root.Flags().Lookup("scope").NoOptDefVal = "all"
	sub := &cobra.Command{
		Use: "doctor", Aliases: []string{"check"}, Short: "Diagnose integrations and `critty install` results",
		Long: "Check integrations.\n\nTargets:\n  claude-code  Claude Code skills that keep their indentation even on narrow terminals\n  codex        Codex skills",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	sub.Flags().String("python", "python3", "Python executable")
	root.AddCommand(sub)
	return root
}

func TestPlainHelpMatchesCobra(t *testing.T) {
	for _, path := range [][]string{nil, {"doctor"}} {
		cmd, _, err := newHelpTestCommand().Find(path)
		if err != nil {
			t.Fatal(err)
		}
		var want bytes.Buffer
		cmd.SetOut(&want)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		if got := plainHelp(cmd); got != want.String() {
			t.Fatalf("plain help for %v = %q, want Cobra's %q", path, got, want.String())
		}
	}
}

func TestStyledHelpLayout(t *testing.T) {
	root := newHelpTestCommand()
	for _, dark := range []bool{true, false} {
		styled := styledHelp(root, dark, 60)
		plain := ansi.Strip(styled)
		for _, want := range []string{
			"critty [file] [flags]", "critty [command]", "doctor",
			"-o, --output string", "--scope string[=\"all\"]", "--launcher stringArray",
		} {
			if !strings.Contains(plain, want) {
				t.Fatalf("dark=%t: help lacks %q:\n%s", dark, want, plain)
			}
		}
		if strings.Contains(plain, "`") || strings.Contains(plain, "Global Flags") {
			t.Fatalf("dark=%t: root help keeps backquotes or lists inherited flags:\n%s", dark, plain)
		}
		for _, line := range strings.Split(plain, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatalf("dark=%t: line exceeds the width: %q", dark, line)
			}
		}
		if !strings.Contains(plain, "custom:EXECUTABLE") || strings.Contains(plain, "desktop-\n") {
			t.Fatalf("dark=%t: wrapping split a hyphenated value:\n%s", dark, plain)
		}
		if !strings.Contains(styled, "\x1b[") {
			t.Fatal("styled help has no styling")
		}
	}

	sub, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(styledHelp(sub, true, 50))
	for _, want := range []string{
		"doctor, check", "  claude-code  Claude Code skills that keep their indentation even on narrow terminals\n",
		"--python string", "\"python3\"", "Global Flags", "--launcher stringArray",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("subcommand help lacks %q:\n%s", want, plain)
		}
	}
}

func TestPagerCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		argv []string
		vars []string
	}{
		{"default less", nil, []string{"less", "-R"}, []string{"LESS=FRX"}},
		{"user LESS", map[string]string{"LESS": "-i"}, []string{"less", "-R"}, nil},
		{"less with flags", map[string]string{"PAGER": "/usr/bin/less -F"}, []string{"/usr/bin/less", "-F", "-R"}, []string{"LESS=FRX"}},
		{"other pager", map[string]string{"PAGER": "more"}, []string{"more"}, nil},
		{"shell pager", map[string]string{"PAGER": "less -F | cat"}, []string{"sh", "-c", "less -F | cat"}, []string{"LESS=FRX"}},
		{"shell pager without less", map[string]string{"PAGER": "$HOME/bin/page"}, []string{"sh", "-c", "$HOME/bin/page"}, nil},
		{"disabled", map[string]string{"PAGER": ""}, nil, nil},
		{"cat", map[string]string{"PAGER": "cat"}, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			argv, vars := pagerCommand(func(key string) (string, bool) { v, ok := tt.env[key]; return v, ok })
			if !slices.Equal(argv, tt.argv) || !slices.Equal(vars, tt.vars) {
				t.Fatalf("pagerCommand = %q, %q; want %q, %q", argv, vars, tt.argv, tt.vars)
			}
		})
	}
}
