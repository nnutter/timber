package timber

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func (x Runtime) autoInstallHKHooks(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string) {
	warn := func(err error) {
		fmt.Fprintf(output, "warning: automatic hk install skipped: %v\n", err)
	}
	settings, err := x.loadConfig()
	if err != nil {
		warn(err)
		return
	}
	if !settings.AutoHKInstall && (settings.AutoHKInstallDecided || !isInteractiveTerminal(input)) {
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
		enabled, err := x.promptAutoHKInstall(input, output, settings, identity)
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
	install := x.command(ctx, "hk", args...)
	install.Dir = worktreePath
	combined, err := install.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("hk install failed: %w: %s", err, strings.TrimSpace(string(combined))))
	}
}

func (x Runtime) promptAutoHKInstall(input io.Reader, output io.Writer, settings config, identity githubIdentity) (bool, error) {
	if _, err := fmt.Fprintln(output, "This worktree qualifies for automatic hk hook installation. Installing hk hooks permits code execution."); err != nil {
		return false, err
	}
	if err := writeGitHubIdentity(output, identity); err != nil {
		return false, err
	}
	reader := bufio.NewReader(input)
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
