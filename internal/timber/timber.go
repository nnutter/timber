package timber

import (
	"github.com/spf13/cobra"

	cliconfig "github.com/nnutter/timber/internal/cli/config"
	"github.com/nnutter/timber/internal/cli/generate"
	clirepo "github.com/nnutter/timber/internal/cli/repo"
	clireview "github.com/nnutter/timber/internal/cli/review"
)

func Commands(runtime Runtime) []*cobra.Command {
	return []*cobra.Command{
		cliconfig.New(runtime),
		NewCreateCommand(runtime),
		NewListCommand(runtime),
		NewPruneCommand(runtime),
		NewRemoveCommand(runtime),
		clirepo.New(
			NewRepoAddCommand(runtime),
			NewRepoImportCommand(runtime),
			NewRepoFixCommand(runtime),
			NewRepoListCommand(runtime),
			NewRepoRemoveCommand(runtime),
			NewRepoRenameCommand(runtime),
		),
		clireview.New(runtime.CurrentDirectory, runtime.HomeDirectory, runtime.ReviewSkillContent),
		NewHerdrCommand(runtime),
		NewSwitchCommand(runtime),
		NewTodoCommand(runtime),
		NewGitCommand(runtime),
		NewTUICommand(runtime),
		generate.New(runtime.xdgDataHome()),
	}
}
