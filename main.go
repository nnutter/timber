package main

import (
	"context"
	_ "embed"
	"os"

	"charm.land/fang/v2"

	"github.com/nnutter/timber/internal/timber"
)

//go:embed skills/timber-todo/SKILL.md
var todoSkillContent string

// version is set via ldflags at build time (e.g. -X main.version=v1.2.3).
var version string

func main() {
	runtime, err := timber.RuntimeFromProcess()
	if err != nil {
		os.Exit(1)
	}
	runtime.TodoSkillContent = todoSkillContent

	if err := fang.Execute(
		context.Background(),
		timber.NewRootCommand(runtime),
		fang.WithVersion(version),
	); err != nil {
		os.Exit(1)
	}
}
