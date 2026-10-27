package main

import (
	"context"
	"os"

	"github.com/nnutter/timber/internal/gitreview/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:], ".", os.Stdin, os.Stdout, os.Stderr, "test"); err != nil {
		os.Exit(1)
	}
}
