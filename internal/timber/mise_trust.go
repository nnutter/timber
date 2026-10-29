package timber

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nnutter/timber/internal/terminal"
)

func (x Runtime) autoTrustMise(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string) {
	warn := func(err error) {
		// Warnings are best effort and must not block worktree creation.
		_, _ = fmt.Fprintf(output, "warning: automatic mise trust skipped: %v\n", err)
	}
	settings, err := x.loadConfig()
	if err != nil {
		warn(err)
		return
	}
	if !settings.AutoMiseTrust && (settings.AutoMiseTrustDecided || !terminal.IsInteractive(input)) {
		return
	}
	configPath := filepath.Join(worktreePath, "mise.toml")
	regular, err := regularRootFile(configPath)
	if err != nil {
		warn(err)
		return
	}
	// Do not trust a symlink that can point outside the new checkout.
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
	if !settings.AutoMiseTrust {
		enabled, err := x.promptAutoMiseTrust(input, output, settings, identity)
		if err != nil {
			warn(err)
			return
		}
		if !enabled {
			return
		}
	}
	trust := x.command(ctx, "mise", "trust", configPath)
	trust.Dir = worktreePath
	combined, err := trust.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("mise trust failed: %w: %s", err, strings.TrimSpace(string(combined))))
	}
}

// regularRootFile reports whether path is a regular file instead of a symlink
// or directory. A missing file is not an error.
func regularRootFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func writeGitHubIdentity(output io.Writer, identity githubIdentity) error {
	organizations := strings.Join(identity.Organizations, ", ")
	if organizations == "" {
		organizations = "none"
	}
	_, err := fmt.Fprintf(output, "GitHub username: %s\nGitHub organizations: %s\n", identity.Username, organizations)
	return err
}

// githubOwnedWorktree reports whether the selected upstream resolves to a
// GitHub repository owned by the cached user or one of their organizations.
func (x Runtime) githubOwnedWorktree(ctx context.Context, repository *Repository, upstream string) (githubIdentity, bool, error) {
	remoteURL, err := repository.upstreamRemoteURL(upstream)
	if err != nil {
		return githubIdentity{}, false, err
	}
	owner := githubRemoteOwner(remoteURL)
	if owner == "" {
		return githubIdentity{}, false, nil
	}
	identity, err := x.githubIdentity(ctx)
	if err != nil {
		return githubIdentity{}, false, err
	}
	if !identity.owns(owner) {
		return githubIdentity{}, false, nil
	}
	return identity, true, nil
}

func (x Runtime) promptAutoMiseTrust(input io.Reader, output io.Writer, settings config, identity githubIdentity) (bool, error) {
	if _, err := fmt.Fprintln(output, "This worktree qualifies for automatic mise trust. Trusting mise configuration permits code execution."); err != nil {
		return false, err
	}
	if err := writeGitHubIdentity(output, identity); err != nil {
		return false, err
	}
	reader := bufio.NewReader(input)
	for {
		if _, err := fmt.Fprint(output, "Enable automatic mise trust for repositories owned by you or your GitHub organizations? [y/N] "); err != nil {
			return false, err
		}
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read mise trust choice: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			settings.AutoMiseTrust = true
		case "", "n", "no":
			settings.AutoMiseTrust = false
		default:
			if _, err := fmt.Fprintln(output, "Enter yes or no."); err != nil {
				return false, err
			}
			continue
		}
		settings.AutoMiseTrustDecided = true
		if err := writeConfig(x.configPath(), settings); err != nil {
			return false, fmt.Errorf("save mise trust choice: %w", err)
		}
		if !settings.AutoMiseTrust {
			_, err := fmt.Fprintln(output, "Automatic mise trust remains disabled. Enable it later with: timber config --auto-mise-trust")
			return false, err
		}
		return true, nil
	}
}

// Resolve the selected start point, including local branches that track a remote.
// Never substitute origin when the selected upstream has no remote provenance.
func (x *Repository) upstreamRemoteURL(upstream string) (string, error) {
	result, err := x.git("rev-parse", "--verify", "--symbolic-full-name", "--end-of-options", upstream)
	if err != nil {
		return "", fmt.Errorf("resolve upstream for mise trust: %w", err)
	}
	ref := result.stdout
	if branch, local := strings.CutPrefix(ref, branchRefPrefix); local {
		tracked, err := x.upstreamReference(branch)
		if err != nil {
			return "", nil
		}
		ref = string(tracked)
	}
	if !strings.HasPrefix(ref, remoteRefPrefix) {
		return "", nil
	}
	remotes, err := x.git("remote")
	if err != nil {
		return "", err
	}
	var selected string
	for remote := range strings.SplitSeq(remotes.stdout, "\n") {
		if remote != "" && strings.HasPrefix(ref, remoteRefPrefix+remote+"/") && len(remote) > len(selected) {
			selected = remote
		}
	}
	if selected == "" {
		return "", nil
	}
	remoteURL, err := x.git("remote", "get-url", "--", selected)
	if err != nil {
		return "", fmt.Errorf("resolve upstream remote for mise trust: %w", err)
	}
	return remoteURL.stdout, nil
}

func githubRemoteOwner(remote string) string {
	var host, repoPath string
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "ssh") {
			return ""
		}
		host, repoPath = parsed.Hostname(), parsed.Path
		if strings.EqualFold(host, "ssh.github.com") && parsed.Scheme != "ssh" {
			return ""
		}
	} else {
		var found bool
		host, repoPath, found = strings.Cut(remote, ":")
		if !found {
			return ""
		}
		if _, hostname, found := strings.Cut(host, "@"); found {
			host = hostname
		}
	}
	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "ssh.github.com") {
		return ""
	}
	owner, repo, found := strings.Cut(strings.TrimPrefix(repoPath, "/"), "/")
	if !found || !githubLoginPattern.MatchString(owner) || repo == "" || repo == "." || repo == ".." || strings.Contains(repo, "/") {
		return ""
	}
	return owner
}
