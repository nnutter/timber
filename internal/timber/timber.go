package timber

import (
	"io"

	"github.com/spf13/cobra"

	cliconfig "github.com/nnutter/timber/internal/cli/config"
	clicreate "github.com/nnutter/timber/internal/cli/create"
	"github.com/nnutter/timber/internal/cli/generate"
	cligit "github.com/nnutter/timber/internal/cli/git"
	clilist "github.com/nnutter/timber/internal/cli/list"
	cliprune "github.com/nnutter/timber/internal/cli/prune"
	cliremove "github.com/nnutter/timber/internal/cli/remove"
	clirepo "github.com/nnutter/timber/internal/cli/repo"
	repoadd "github.com/nnutter/timber/internal/cli/repo/add"
	repofix "github.com/nnutter/timber/internal/cli/repo/fix"
	repoimport "github.com/nnutter/timber/internal/cli/repo/import"
	repolist "github.com/nnutter/timber/internal/cli/repo/list"
	reporemove "github.com/nnutter/timber/internal/cli/repo/remove"
	reporename "github.com/nnutter/timber/internal/cli/repo/rename"
	clireview "github.com/nnutter/timber/internal/cli/review"
	cliswitch "github.com/nnutter/timber/internal/cli/switch"
	clitodo "github.com/nnutter/timber/internal/cli/todo"
)

func Commands(runtime Runtime) []*cobra.Command {
	return []*cobra.Command{
		cliconfig.New(runtime),
		clicreate.New(clicreate.Dependencies{
			Create:     runtime.CreateManagedWorktree,
			ReportPath: runtime.reportCreatedWorktreePath,
			Complete:   runtime.completeCreateArgs,
		}),
		clilist.New(clilist.Dependencies{
			List:     runtime.ListWorktrees,
			Complete: runtime.completeRepoQualifiers,
		}),
		cliprune.New(cliprune.Dependencies{
			Prune: func(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool) error {
				return runtime.PruneWorktrees(input, output, repoArg, prompt, dryRun, huhWorktreePrompter{})
			},
			Complete: runtime.completeRepoQualifiers,
		}),
		cliremove.New(cliremove.Dependencies{
			Remove:       runtime.RemoveWorktree,
			Complete:     runtime.completeQualifiedWorktreeNames,
			RenderStatus: func(message string) string { return statusStyle.Render(message) },
			RenderWarning: func(message string) string {
				return warningStyle.Render(message)
			},
		}),
		clirepo.New(
			repoadd.New(repoadd.Dependencies{
				Add:          runtime.AddRepo,
				RenderStatus: func(message string) string { return statusStyle.Render(message) },
			}),
			repoimport.New(repoimport.Dependencies{
				Import: runtime.ImportRepo,
			}),
			repofix.New(repofix.Dependencies{
				Fix:          runtime.FixRepos,
				RenderStatus: func(message string) string { return statusStyle.Render(message) },
			}),
			repolist.New(runtime.ListRepos),
			reporemove.New(runtime.removeRepo, runtime.completeRegisteredRepoNames, func(message string) string { return statusStyle.Render(message) }),
			reporename.New(reporename.Dependencies{
				Rename:       runtime.RenameRepo,
				Complete:     runtime.completeRepoRenameArguments,
				RenderStatus: func(message string) string { return statusStyle.Render(message) },
			}),
		),
		clireview.New(runtime.CurrentDirectory, runtime.HomeDirectory, runtime.ReviewSkillContent),
		herdrCommand(runtime),
		cliswitch.New(cliswitch.Dependencies{
			Resolve:    runtime.ResolveSwitchWorktree,
			Create:     runtime.CreateManagedWorktree,
			ReportPath: runtime.reportSwitchWorktreePath,
			Complete:   runtime.completeQualifiedWorktreeNames,
		}),
		todoCommand(runtime),
		cligit.New(cligit.Dependencies{
			CurrentDirectory:   runtime.CurrentDirectory,
			Command:            runtime.command,
			ResolveSelector:    runtime.ResolveGitSelector,
			ResolveGitDir:      runtime.GitDirForDirectory,
			CompleteWorktrees:  runtime.completeQualifiedWorktreeNames,
			CompleteRepoSuffix: runtime.completeRepoSuffix,
		}),
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
