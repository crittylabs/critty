package main

import (
	"os"

	"github.com/crittylabs/critty/v2/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
