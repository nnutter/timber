package timber

import (
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/generate"
)

func Commands(runtime Runtime) []*cobra.Command {
	return []*cobra.Command{
		NewConfigCommand(runtime),
		NewCreateCommand(runtime),
		NewListCommand(runtime),
		NewPruneCommand(runtime),
		NewRemoveCommand(runtime),
		NewRepoCommand(runtime),
		NewReviewCommand(runtime),
		NewHerdrCommand(runtime),
		NewSwitchCommand(runtime),
		NewTodoCommand(runtime),
		NewGitCommand(runtime),
		NewTUICommand(runtime),
		generate.New(runtime.xdgDataHome()),
	}
}
