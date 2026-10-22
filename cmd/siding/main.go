package main

import (
	"os"

	"github.com/TheBlackhowling/siding/internal/cli"
)

var exitFunc = os.Exit

func main() {
	if err := cli.Execute(); err != nil {
		exitFunc(1)
	}
}
