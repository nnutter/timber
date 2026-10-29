package timber

import (
	"github.com/spf13/cobra"

	cliconfig "github.com/nnutter/timber/internal/cli/config"
	"github.com/nnutter/timber/internal/cli/generate"
	clirepo "github.com/nnutter/timber/internal/cli/repo"
	reporemove "github.com/nnutter/timber/internal/cli/repo/remove"
	clireview "github.com/nnutter/timber/internal/cli/review"
	clitodo "github.com/nnutter/timber/internal/cli/todo"
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
			reporemove.New(runtime.removeRepo, runtime.completeRegisteredRepoNames, func(message string) string { return statusStyle.Render(message) }),
			NewRepoRenameCommand(runtime),
		),
		clireview.New(runtime.CurrentDirectory, runtime.HomeDirectory, runtime.ReviewSkillContent),
		herdrCommand(runtime),
		NewSwitchCommand(runtime),
		todoCommand(runtime),
		NewGitCommand(runtime),
		NewTUICommand(runtime),
		generate.New(runtime.xdgDataHome()),
	}
}

// todoCommand connects the TODO CLI to worktree selection and process state.
func todoCommand(runtime Runtime) *cobra.Command {
	return clitodo.New(clitodo.Dependencies{
		CurrentDirectory: runtime.CurrentDirectory,
		HomeDirectory:    runtime.HomeDirectory,
		Environment:      runtime.Environment,
		SkillContent:     runtime.TodoSkillContent,
		Command:          runtime.command,
		Complete:         runtime.completeQualifiedWorktreeNames,
		RenderStatus:     func(message string) string { return statusStyle.Render(message) },
		ResolveGitDir:    runtime.resolveTodoGitDir,
	})
}
