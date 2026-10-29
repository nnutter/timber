package timber

import (
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

func (x Runtime) autoTrustMise(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter) {
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
		if prompter == nil {
			warn(errors.New("automatic setup prompt is not configured"))
			return
		}
		enabled, err := prompter.Choose(input, output, SetupMiseTrust, SetupIdentity{Username: identity.Username, Organizations: identity.Organizations})
		if err != nil {
			warn(err)
			return
		}
		settings.AutoMiseTrust = enabled
		settings.AutoMiseTrustDecided = true
		if err := writeConfig(x.configPath(), settings); err != nil {
			warn(fmt.Errorf("save mise trust choice: %w", err))
			return
		}
		if !enabled {
			if err := prompter.ReportDisabled(output, SetupMiseTrust); err != nil {
				warn(err)
			}
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
