package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"

	"github.com/nnutter/timber/internal/cli"
	"github.com/nnutter/timber/internal/cli/root"
	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/process"

	"charm.land/fang/v2"

	"github.com/nnutter/timber/internal/timber"
)

//go:embed skills/timber-todo/SKILL.md
var todoSkillContent string

//go:embed skills/timber/SKILL.md
var skillContent string

//go:embed skills/timber-review/SKILL.md
var reviewSkillContent string

// version is set via ldflags at build time (e.g. -X main.version=v1.2.3).
var version string

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := process.InterruptContext(context.Background())
	defer stop()
	runtime, err := timber.RuntimeFromProcess(ctx)
	if err != nil {
		return 1
	}
	runtime.TodoSkillContent = todoSkillContent
	runtime.SkillContent = skillContent
	runtime.ReviewSkillContent = reviewSkillContent

	command := root.New(runtime.HomeDirectory, runtime.SkillContent, cli.Commands(runtime)...)
	expand := root.ConfigureAliases(command, runtime.ReadConfig)
	args, err := expand(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	command.SetArgs(args)
	if err := fang.Execute(
		ctx,
		command,
		fang.WithVersion(version),
		fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
			if len(args) > 0 && isSame(args[0], "review") && display.WantsJSON(args[1:]) {
				failed, _, _ := command.Find(args)
				name := "review"
				if failed != nil {
					name = failed.Name()
				}
				_ = display.WriteJSON(command.OutOrStdout(), display.ErrorResult{Version: 1, Command: name, Status: "error", Error: err.Error()})
				return
			}
			fang.DefaultErrorHandler(w, styles, err)
		}),
	); err != nil {
		return 1
	}
	if ctx.Err() != nil {
		return 1
	}
	return 0
}
