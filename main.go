package main

import (
	"context"
	"os"

	"charm.land/fang/v2"

	"github.com/nnutter/timber/internal/timber"
)

// version is set via ldflags at build time (e.g. -X main.version=v1.2.3).
var version string

func main() {
	runtime, err := timber.RuntimeFromProcess()
	if err != nil {
		os.Exit(1)
	}

	if err := fang.Execute(
		context.Background(),
		timber.NewRootCommand(runtime),
		fang.WithVersion(version),
	); err != nil {
		os.Exit(1)
	}
}
