package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootShellCompletion(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"aaa.md", "review notes.md", "view", ".hidden.md", "re\tinvalid"} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir("review-docs", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("review-docs", "review-link"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		args      []string
		contains  []string
		directive cobra.ShellCompDirective
	}{
		{"commands and files", []string{""}, []string{"review\treview command", "aaa.md", "review notes.md", "review-docs/", "review-link/", "./view"}, cobra.ShellCompDirectiveKeepOrder | cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace},
		{"shared prefix", []string{"re"}, []string{"review\treview command", "review notes.md", "review-docs/", "review-link/"}, cobra.ShellCompDirectiveKeepOrder | cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace},
		{"ordinary file prefix", []string{"aaa"}, nil, cobra.ShellCompDirectiveDefault},
		{"explicit path", []string{filepath.FromSlash("./re")}, nil, cobra.ShellCompDirectiveDefault},
		{"directory contents", []string{"review-docs/"}, nil, cobra.ShellCompDirectiveDefault},
		{"hidden file prefix", []string{".h"}, nil, cobra.ShellCompDirectiveDefault},
		{"home directory", []string{"~/"}, nil, cobra.ShellCompDirectiveDefault},
		{"argument already given", []string{"aaa.md", ""}, nil, cobra.ShellCompDirectiveNoFileComp},
		{"flag", []string{"--sam"}, []string{"--sample"}, cobra.ShellCompDirectiveNoFileComp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "critty", Args: cobra.MaximumNArgs(1), Run: func(*cobra.Command, []string) {}, ValidArgsFunction: rootCmd.ValidArgsFunction}
			cmd.AddCommand(&cobra.Command{Use: "review", Short: "review command", Run: func(*cobra.Command, []string) {}})
			cmd.AddCommand(&cobra.Command{Use: "view", Run: func(*cobra.Command, []string) {}})
			cmd.Flags().Bool("sample", false, "sample flag")
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(append([]string{"__complete"}, tc.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			got := output.String()
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("completion missing %q: %s", want, got)
				}
			}
			if !strings.HasSuffix(got, fmt.Sprintf(":%d\n", tc.directive)) || strings.Contains(got, ".hidden.md") || strings.Contains(got, "invalid") {
				t.Fatalf("unexpected completion: %q", got)
			}
			if i := strings.Index(got, "review\t"); i >= 0 {
				if j := strings.Index(got, "review notes.md"); j >= 0 && i > j {
					t.Fatalf("file precedes command: %s", got)
				}
			}
		})
	}
}

func TestTransientCommandSelection(t *testing.T) {
	oldSession, oldDemo := rootSession, reviewDemo
	oldDiff, oldCode := reviewDiff, reviewCode
	t.Cleanup(func() {
		rootSession, reviewDemo = oldSession, oldDemo
		reviewDiff, reviewCode = oldDiff, oldCode
	})
	for _, tc := range []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		session string
		demo    bool
		want    bool
	}{
		{"bare Git review", rootCmd, nil, "", false, false},
		{"implicit document view", rootCmd, []string{"notes.md:3"}, "", false, true},
		{"explicit document review", reviewCmd, []string{"notes.md"}, "", false, false},
		{"legacy document resume", rootCmd, []string{"notes.md"}, "session", false, false},
		{"explicit view", viewCmd, nil, "", false, true},
		{"demo review", reviewCmd, []string{"notes.md"}, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootSession, reviewDemo = tc.session, tc.demo
			if got := transientReview(tc.cmd, tc.args); got != tc.want {
				t.Fatalf("transientReview = %t, want %t", got, tc.want)
			}
		})
	}
	rootSession, reviewDemo = "", false
	for _, code := range []bool{false, true} {
		reviewDiff, reviewCode = "-", code
		if defaultFileView([]string{"input.diff"}) {
			t.Fatal("treated a supplied diff as a document view")
		}
	}
	reviewDiff, reviewCode = "", true
	if defaultFileView([]string{"notes.md"}) {
		t.Fatal("treated explicit code mode as a document view")
	}
}
