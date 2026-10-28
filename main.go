package main

import (
	"context"
	_ "embed"
	"io"
	"os"

	"github.com/nnutter/timber/internal/display"

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
	runtime, err := timber.RuntimeFromProcess()
	if err != nil {
		os.Exit(1)
	}
	runtime.TodoSkillContent = todoSkillContent
	runtime.SkillContent = skillContent
	runtime.ReviewSkillContent = reviewSkillContent

	root := timber.NewRootCommand(runtime)
	if err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(version),
		fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
			args := os.Args[1:]
			if len(args) > 0 && args[0] == "review" && display.WantsJSON(args[1:]) {
				failed, _, _ := root.Find(args)
				name := "review"
				if failed != nil {
					name = failed.Name()
				}
				_ = display.WriteJSON(root.OutOrStdout(), display.ErrorResult{Version: 1, Command: name, Status: "error", Error: err.Error()})
				return
			}
			fang.DefaultErrorHandler(w, styles, err)
		}),
	); err != nil {
		os.Exit(1)
	}
}
