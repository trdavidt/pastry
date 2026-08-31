package main

import (
	"os"

	"pastry/cli"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
