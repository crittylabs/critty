package main

import (
	"os"

	"github.com/knu/tcrit/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
