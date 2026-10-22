package timber

import (
	"bufio"
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
	if !settings.AutoHKInstall && (settings.AutoHKInstallDecided || !isInteractiveTerminal(command.InOrStdin())) {
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
	identity, owned, err := x.githubOwnedWorktree(command, repository, upstream)
	if err != nil {
		warn(err)
		return
	}
	if !owned {
		return
	}
	if !settings.AutoHKInstall {
		enabled, err := x.promptAutoHKInstall(command, settings, identity)
		if err != nil {
			warn(err)
			return
		}
		if !enabled {
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
	install := x.command(command.Context(), "hk", args...)
	install.Dir = worktreePath
	output, err := install.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("hk install failed: %w: %s", err, strings.TrimSpace(string(output))))
	}
}

func (x Runtime) promptAutoHKInstall(command *cobra.Command, settings config, identity githubIdentity) (bool, error) {
	output := command.ErrOrStderr()
	if _, err := fmt.Fprintln(output, "This worktree qualifies for automatic hk hook installation. Installing hk hooks permits code execution."); err != nil {
		return false, err
	}
	if err := writeGitHubIdentity(output, identity); err != nil {
		return false, err
	}
	reader := bufio.NewReader(command.InOrStdin())
	for {
		if _, err := fmt.Fprint(output, "Enable automatic hk hook installation for repositories owned by you or your GitHub organizations? [y/N] "); err != nil {
			return false, err
		}
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read hk install choice: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			settings.AutoHKInstall = true
		case "", "n", "no":
			settings.AutoHKInstall = false
		default:
			if _, err := fmt.Fprintln(output, "Enter yes or no."); err != nil {
				return false, err
			}
			continue
		}
		settings.AutoHKInstallDecided = true
		if err := writeConfig(x.configPath(), settings); err != nil {
			return false, fmt.Errorf("save hk install choice: %w", err)
		}
		if !settings.AutoHKInstall {
			_, err := fmt.Fprintln(output, "Automatic hk hook installation remains disabled. Enable it later with: timber config --auto-hk-install")
			return false, err
		}
		return true, nil
	}
}
