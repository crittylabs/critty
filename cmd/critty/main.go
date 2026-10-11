package main

import (
	"os"

	"github.com/crittylabs/critty/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
