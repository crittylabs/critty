package main

import (
	"fmt"
	"os"

	"github.com/knu/tcrit/internal/codexwrapper"
)

func main() {
	if err := codexwrapper.Run(os.Args[1:], true); err != nil {
		fmt.Fprintln(os.Stderr, "critty codex:", err)
		os.Exit(1)
	}
}
