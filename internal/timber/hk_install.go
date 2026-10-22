package timber

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (x Runtime) autoInstallHKHooks(command *cobra.Command, repository *Repository, upstream, worktreePath string) {
	warn := func(err error) {
		fmt.Fprintf(command.ErrOrStderr(), "warning: automatic hk install skipped: %v\n", err)
	}
	settings, err := x.loadConfig()
	if err != nil {
		warn(err)
		return
	}
	if !settings.AutoHKInstall {
		return
	}
	regular, err := regularRootFile(filepath.Join(worktreePath, "hk.pkl"))
	if err != nil {
		warn(err)
		return
	}
	if !regular {
		return
	}
	_, owned, err := x.githubOwnedWorktree(command, repository, upstream)
	if err != nil {
		warn(err)
		return
	}
	if !owned {
		return
	}
	args := []string{"install"}
	hasMise, err := regularRootFile(filepath.Join(worktreePath, "mise.toml"))
	if err != nil {
		warn(err)
		return
	}
	if hasMise {
		args = append(args, "--mise")
	}
	install := x.command(command.Context(), "hk", args...)
	install.Dir = worktreePath
	output, err := install.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("hk install failed: %w: %s", err, strings.TrimSpace(string(output))))
	}
}
