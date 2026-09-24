package main

import (
	"os"

	"github.com/AnthonyPoschen/genos-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Options{}))
}
