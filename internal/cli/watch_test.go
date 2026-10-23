package cli

import (
	"slices"
	"testing"

	"github.com/crittylabs/critty/internal/git"
	"github.com/crittylabs/critty/internal/review"
	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"
)

func TestWatchFlagAndLauncher(t *testing.T) {
	oldWatch, oldExec := reviewWatch, resolveExec
	t.Cleanup(func() { reviewWatch, resolveExec = oldWatch, oldExec })
	for _, cmd := range []*cobra.Command{rootCmd, reviewCmd, viewCmd, planCmd, tuiCmd} {
		flag := cmd.Flags().Lookup("watch")
		if flag == nil || flag.DefValue != "false" {
			t.Fatalf("%s: missing default-off --watch", cmd.Name())
		}
	}
	resolveExec = func() (string, error) { return "/path with spaces/critty", nil }
	reviewWatch = true
	command, err := buildTUICommand(&reviewMode{sessionKey: "session"}, "tmux")
	if err != nil {
		t.Fatal(err)
	}
	args, err := shellwords.Parse(command)
	if err != nil || !slices.Contains(args, "--watch") {
		t.Fatalf("command=%q err=%v", command, err)
	}
	for _, mode := range []*reviewMode{{patch: &git.Patch{}}, {planSlug: "plan"}, {source: &git.ReviewSource{Scope: "range"}}} {
		if validateWatchMode(mode) == nil {
			t.Fatal("accepted snapshot-only watch")
		}
		mode.documents = []review.AttachedDocument{{Source: "document.md"}}
		if err := validateWatchMode(mode); err != nil {
			t.Fatal(err)
		}
	}
}
