package main

import (
	"os"

	"github.com/maskedsyntax/goggles/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Stdin, os.Stdout, os.Stderr, os.Args[1:]))
}
