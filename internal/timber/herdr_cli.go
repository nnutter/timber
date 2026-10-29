package timber

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	cliherdr "github.com/nnutter/timber/internal/cli/herdr"
	"github.com/nnutter/timber/internal/cli/herdr/install"
	"github.com/nnutter/timber/internal/cli/herdr/space"
)

func herdrCommand(runtime Runtime) *cobra.Command {
	selection := repoSelection{runtime: runtime}
	renderStatus := func(message string) string { return statusStyle.Render(message) }
	installDeps := install.Dependencies{
		Directory:  runtime.herdrPluginInstallDirectory(),
		ConfigFile: runtime.herdrConfigFilePath(),
		Link: func(ctx context.Context, destination string) error {
			_, err := runtime.runHerdr(ctx, "plugin", "link", destination, "--enabled")
			return err
		},
		RenderStatus: renderStatus,
	}
	spaceDeps := space.Dependencies{
		Resolve: func(input io.Reader, raw string) (space.Worktree, error) {
			_, _, worktree, err := selection.resolveQualifiedWorktree(input, raw)
			return space.Worktree{Repo: worktree.Repo, Name: worktree.Name, Path: worktree.Path}, err
		},
		Open: func(ctx context.Context, worktree space.Worktree) error {
			return runtime.openHerdrSpace(ctx, managedWorktree{Repo: worktree.Repo, Name: worktree.Name, Path: worktree.Path})
		},
		Define: func(ctx context.Context, worktree space.Worktree) error {
			return runtime.defineCurrentHerdrSpace(ctx, managedWorktree{Repo: worktree.Repo, Name: worktree.Name, Path: worktree.Path})
		},
		Complete:     runtime.completeQualifiedWorktreeNames,
		RenderStatus: renderStatus,
	}
	return cliherdr.New(installDeps, spaceDeps)
}
