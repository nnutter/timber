package timber

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/nnutter/timber/internal/terminal"
)

func (x Runtime) autoInstallHKHooks(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter, reportWarning func(io.Writer, SetupKind, error) error) {
	warn := func(err error) {
		if reportWarning != nil {
			_ = reportWarning(output, SetupHKInstall, err)
		}
	}
	settings, err := x.loadConfig()
	if err != nil {
		warn(err)
		return
	}
	if !settings.AutoHKInstall && (settings.AutoHKInstallDecided || !terminal.IsInteractive(input)) {
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
	identity, owned, err := x.githubOwnedWorktree(ctx, repository, upstream)
	if err != nil {
		warn(err)
		return
	}
	if !owned {
		return
	}
	if !settings.AutoHKInstall {
		if prompter == nil {
			warn(errors.New("automatic setup prompt is not configured"))
			return
		}
		enabled, err := prompter.Choose(input, output, SetupHKInstall, SetupIdentity{Username: identity.Username, Organizations: identity.Organizations})
		if err != nil {
			warn(err)
			return
		}
		settings.AutoHKInstall = enabled
		settings.AutoHKInstallDecided = true
		if err := writeConfig(x.configPath(), settings); err != nil {
			warn(fmt.Errorf("save hk install choice: %w", err))
			return
		}
		if !enabled {
			if err := prompter.ReportDisabled(output, SetupHKInstall); err != nil {
				warn(err)
			}
			return
		}
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
	install := x.command(ctx, "hk", args...)
	install.Dir = worktreePath
	combined, err := install.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("hk install failed: %w: %s", err, strings.TrimSpace(string(combined))))
	}
}
