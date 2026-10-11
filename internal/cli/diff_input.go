package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/crittylabs/critty/internal/git"
)

func addDiffFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&reviewNull, "null", false, "review only attachments, without Git or diff input")
	cmd.Flags().StringVar(&reviewDiff, "diff", "", "read a Git unified diff from FILE or stdin (-)")
	cmd.Flags().Lookup("diff").NoOptDefVal = "-"
	cmd.PreRunE = validateScopeFlags
}

func readDiff(path string) (*git.Patch, error) {
	if path == "-" {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return nil, fmt.Errorf("--diff requires a unified diff on stdin")
		}
		return git.ParsePatch(os.Stdin)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	patch, parseErr := git.ParsePatch(f)
	closeErr := f.Close()
	if parseErr != nil {
		return nil, parseErr
	}
	return patch, closeErr
}
