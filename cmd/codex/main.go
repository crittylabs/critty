package main

import (
	"fmt"
	"os"

	"github.com/crittylabs/critty/v2/internal/codexwrapper"
)

func main() {
	if err := codexwrapper.Run(os.Args[1:], true); err != nil {
		fmt.Fprintln(os.Stderr, "critty codex:", err)
		os.Exit(1)
	}
}
