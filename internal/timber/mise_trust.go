package timber

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (x Runtime) autoTrustMise(command *cobra.Command, repository *Repository, upstream, worktreePath string) {
	warn := func(err error) {
		fmt.Fprintf(command.ErrOrStderr(), "warning: automatic mise trust skipped: %v\n", err)
	}
	settings, err := x.loadConfig()
	if err != nil {
		warn(err)
		return
	}
	if !settings.AutoMiseTrust && (settings.AutoMiseTrustDecided || !isInteractiveTerminal(command.InOrStdin())) {
		return
	}
	configPath := filepath.Join(worktreePath, "mise.toml")
	info, err := os.Lstat(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		warn(err)
		return
	}
	// Do not trust a symlink that can point outside the new checkout.
	if !info.Mode().IsRegular() {
		return
	}
	remoteURL, err := repository.upstreamRemoteURL(upstream)
	if err != nil {
		warn(err)
		return
	}
	owner := githubRemoteOwner(remoteURL)
	if owner == "" {
		return
	}
	identity, err := x.githubIdentity(command.Context())
	if err != nil {
		warn(err)
		return
	}
	if !identity.owns(owner) {
		return
	}
	if !settings.AutoMiseTrust {
		enabled, err := x.promptAutoMiseTrust(command, settings, identity)
		if err != nil {
			warn(err)
			return
		}
		if !enabled {
			return
		}
	}
	trust := x.command(command.Context(), "mise", "trust", configPath)
	trust.Dir = worktreePath
	output, err := trust.CombinedOutput()
	if err != nil {
		warn(fmt.Errorf("mise trust failed: %w: %s", err, strings.TrimSpace(string(output))))
	}
}

func (x Runtime) promptAutoMiseTrust(command *cobra.Command, settings config, identity githubIdentity) (bool, error) {
	output := command.ErrOrStderr()
	if _, err := fmt.Fprintln(output, "This worktree qualifies for automatic mise trust. Trusting mise configuration permits code execution."); err != nil {
		return false, err
	}
	organizations := strings.Join(identity.Organizations, ", ")
	if organizations == "" {
		organizations = "none"
	}
	if _, err := fmt.Fprintf(output, "GitHub username: %s\nGitHub organizations: %s\n", identity.Username, organizations); err != nil {
		return false, err
	}
	reader := bufio.NewReader(command.InOrStdin())
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
