// Package codexwrapper registers the invoking terminal and transparently executes Codex.
package codexwrapper

import (
	"fmt"
	"os"
	"syscall"

	"github.com/crittylabs/critty/internal/terminalbinding"
)

// Run keeps Codex arguments, environment, working directory and exit status unchanged.
func Run(args []string, wrapped bool) error {
	bin, err := findCodex(wrapped)
	if err != nil {
		return err
	}
	target, err := terminalbinding.FromEnvironment()
	if err == nil && target.Kind != "" {
		err = terminalbinding.DefaultStore().Register(target)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "critty codex: terminal registration failed: %v\n", err)
	}
	return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
}
