// Package cli assembles the Timber command tree. It depends on the
// application packages for operations and on the command packages for
// Cobra wiring. Application packages must not import this package.
package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	cliComplete "github.com/nnutter/timber/internal/cli/complete"
	cliconfig "github.com/nnutter/timber/internal/cli/config"
	clicreate "github.com/nnutter/timber/internal/cli/create"
	"github.com/nnutter/timber/internal/cli/generate"
	cligit "github.com/nnutter/timber/internal/cli/git"
	cliherdr "github.com/nnutter/timber/internal/cli/herdr"
	"github.com/nnutter/timber/internal/cli/herdr/install"
	"github.com/nnutter/timber/internal/cli/herdr/space"
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
	clitui "github.com/nnutter/timber/internal/cli/tui"
	"github.com/nnutter/timber/internal/cli/worktreeoutput"
	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/timber"
)

// Commands assembles the Timber command tree from application operations.
func Commands(runtime timber.Runtime) []*cobra.Command {
	runtime.RepoPrompter = bubbleteaRepoPrompter{}
	renderStatus := func(message string) string { return display.StatusStyle.Render(message) }
	renderWarning := func(message string) string { return display.WarningStyle.Render(message) }
	createReporters := timber.CreateReporters{
		SetupPrompt:  setupPrompter{},
		SetupWarning: reportSetupWarning,
		Created: func(output io.Writer, path string) error {
			return worktreeoutput.ReportCreated(output, renderStatus, path)
		},
		HerdrOpened: func(output io.Writer, name string) error {
			return worktreeoutput.ReportHerdrOpened(output, renderStatus, name)
		},
	}
	create := func(ctx context.Context, input io.Reader, output io.Writer, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string, herdr, noHerdr bool) (string, bool, error) {
		return runtime.CreateManagedWorktree(ctx, input, output, repoName, raw, explicitPath, hasExplicitPath, upstream, herdr, noHerdr, createReporters)
	}
	return []*cobra.Command{
		cliconfig.New(runtime),
		clicreate.New(clicreate.Dependencies{
			Create:     create,
			ReportPath: runtime.ReportCreatedWorktreePath,
			Complete:   cliComplete.CreateArgs(runtime),
		}),
		clilist.New(clilist.Dependencies{
			List: func(ctx context.Context, stdout io.Writer, repoArg string, pullRequests, location, jsonOutput bool, sortMode string) error {
				return runtime.ListWorktrees(ctx, stdout, repoArg, pullRequests, location, jsonOutput, sortMode, timber.ListRenderers{
					Table: clilist.ReportTable,
					JSON:  clilist.ReportJSON,
				})
			},
			Complete: cliComplete.RepoQualifiers(runtime),
		}),
		cliprune.New(cliprune.Dependencies{
			Prune: func(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool) error {
				return runtime.PruneWorktrees(input, output, repoArg, prompt, dryRun, huhWorktreePrompter{}, timber.PruneReporters{
					DryRun: func(output io.Writer, name, repo, path string) error {
						return worktreeoutput.ReportPruneDryRun(output, renderStatus, name, repo, path)
					},
					Removal: func(output io.Writer, name, shortHash, path string, current bool) error {
						return worktreeoutput.ReportRemoval(output, renderStatus, renderWarning, name, shortHash, path, current)
					},
				})
			},
			Complete: cliComplete.RepoQualifiers(runtime),
		}),
		cliremove.New(cliremove.Dependencies{
			Remove:        runtime.RemoveWorktree,
			Complete:      cliComplete.QualifiedWorktreeNames(runtime),
			RenderStatus:  renderStatus,
			RenderWarning: renderWarning,
		}),
		clirepo.New(
			repoadd.New(repoadd.Dependencies{
				Add:          runtime.AddRepo,
				RenderStatus: renderStatus,
			}),
			repoimport.New(repoimport.Dependencies{
				Import:        runtime.ImportRepo,
				HomeDirectory: runtime.HomeDirectory,
				RenderStatus:  renderStatus,
				RenderWarning: renderWarning,
			}),
			repofix.New(repofix.Dependencies{
				Fix:          runtime.FixRepos,
				RenderStatus: renderStatus,
			}),
			repolist.New(runtime.ListRepos),
			reporemove.New(runtime.RemoveRepo, cliComplete.RegisteredRepoNames(runtime), renderStatus),
			reporename.New(reporename.Dependencies{
				Rename:       runtime.RenameRepo,
				Complete:     cliComplete.RepoRenameArguments(runtime),
				RenderStatus: renderStatus,
			}),
		),
		clireview.New(runtime.CurrentDirectory, runtime.HomeDirectory, runtime.ReviewSkillContent),
		herdrCommand(runtime),
		cliswitch.New(cliswitch.Dependencies{
			Resolve:    runtime.ResolveSwitchWorktree,
			Create:     create,
			ReportPath: runtime.ReportSwitchWorktreePath,
			Complete:   cliComplete.QualifiedWorktreeNames(runtime),
		}),
		todoCommand(runtime),
		cligit.New(cligit.Dependencies{
			CurrentDirectory:   runtime.CurrentDirectory,
			Command:            runtime.NewCommand,
			ResolveSelector:    runtime.ResolveGitSelector,
			ResolveGitDir:      runtime.GitDirForDirectory,
			CompleteWorktrees:  cliComplete.QualifiedWorktreeNames(runtime),
			CompleteRepoSuffix: cliComplete.RepoSuffix(runtime),
		}),
		clitui.New(clitui.Dependencies{
			Run: func(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool) error {
				return runtime.RunTUI(ctx, input, output, stdout, herdr, noHerdr, noTitle, bubbleteaCreateWizardPrompter{}, createReporters)
			},
		}),
		generate.New(runtime.DataHome),
	}
}

// todoCommand connects the TODO CLI to worktree selection and process state.
func todoCommand(runtime timber.Runtime) *cobra.Command {
	return clitodo.New(clitodo.Dependencies{
		CurrentDirectory: runtime.CurrentDirectory,
		HomeDirectory:    runtime.HomeDirectory,
		Environment:      runtime.Environment,
		SkillContent:     runtime.TodoSkillContent,
		Command:          runtime.NewCommand,
		Complete:         cliComplete.QualifiedWorktreeNames(runtime),
		RenderStatus:     func(message string) string { return display.StatusStyle.Render(message) },
		ResolveGitDir:    runtime.ResolveTodoGitDir,
	})
}

func herdrCommand(runtime timber.Runtime) *cobra.Command {
	renderStatus := func(message string) string { return display.StatusStyle.Render(message) }
	installDeps := install.Dependencies{
		Directory:      runtime.HerdrPluginDirectory(),
		ConfigFile:     runtime.HerdrConfigFile(),
		KeybindingTOML: timber.HerdrKeybindingTOML,
		Link: func(ctx context.Context, destination string) error {
			return runtime.LinkHerdrPlugin(ctx, destination)
		},
		RenderStatus: renderStatus,
	}
	spaceDeps := space.Dependencies{
		Resolve: func(input io.Reader, raw string) (space.Worktree, error) {
			repo, name, path, err := runtime.ResolveHerdrWorktree(input, raw)
			return space.Worktree{Repo: repo, Name: name, Path: path}, err
		},
		Open: func(ctx context.Context, worktree space.Worktree) error {
			return runtime.OpenHerdrSpace(ctx, worktree.Repo, worktree.Name, worktree.Path)
		},
		Define: func(ctx context.Context, worktree space.Worktree) error {
			return runtime.DefineHerdrSpace(ctx, worktree.Repo, worktree.Name, worktree.Path)
		},
		Complete:     cliComplete.QualifiedWorktreeNames(runtime),
		RenderStatus: renderStatus,
	}
	return cliherdr.New(installDeps, spaceDeps)
}
