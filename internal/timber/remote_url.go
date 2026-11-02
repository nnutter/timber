package timber

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// parseGitHubShortForm reports whether input is a schema-less GitHub short
// form (owner/repo, optionally with a .git suffix) and returns the
// normalized owner/repo without the suffix.
func parseGitHubShortForm(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	switch {
	case strings.Contains(trimmed, "://"),
		strings.HasPrefix(trimmed, "git@"),
		strings.HasPrefix(trimmed, "/"),
		strings.HasPrefix(trimmed, "."),
		strings.HasPrefix(trimmed, "~"),
		strings.Contains(trimmed, ":"):
		return "", false
	}
	owner, repo, found := strings.Cut(trimmed, "/")
	if !found || isEmpty(owner) || isEmpty(repo) || strings.Contains(repo, "/") {
		return "", false
	}
	repo = strings.TrimSuffix(repo, ".git")
	if repo == "" {
		return "", false
	}
	return owner + "/" + repo, true
}

// resolveRemoteURL maps user input to a git remote URL.
//
// Schema-less GitHub short forms (e.g. "nnutter/timber") become
// git@github.com:<owner>/<repo> without a .git suffix. Absolute URLs,
// SSH forms, and local paths pass through unchanged.
func resolveRemoteURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("repository URL is required")
	}
	return classifiedRemoteURL(trimmed), nil
}

func classifiedRemoteURL(trimmed string) string {
	switch {
	case strings.Contains(trimmed, "://"):
		return trimmed
	case strings.HasPrefix(trimmed, "git@"):
		return trimmed
	case strings.HasPrefix(trimmed, "github.com:"):
		return "git@" + trimmed
	case strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "."):
		return trimmed
	case strings.HasPrefix(trimmed, "~"):
		return trimmed
	case looksLikeSSHShorthand(trimmed):
		return trimmed
	default:
		return githubRemoteURL(trimmed)
	}
}

func githubRemoteURL(trimmed string) string {
	if short, ok := parseGitHubShortForm(trimmed); ok {
		return "git@github.com:" + short
	}
	return "https://github.com/" + strings.TrimPrefix(trimmed, "/")
}

func looksLikeSSHShorthand(input string) bool {
	// host:path without scheme, e.g. gitlab.com:group/repo.git
	if strings.Contains(input, "://") {
		return false
	}
	host, repoPath, found := strings.Cut(input, ":")
	if !found || isEmpty(host) || isEmpty(repoPath) {
		return false
	}
	return !strings.Contains(host, "/") && strings.Contains(host, ".")
}

// remoteHostAndPath splits a remote URL into its host and repository path.
// It reports false when the remote names no host, such as a filesystem path.
func remoteHostAndPath(remoteURL string) (host, repoPath string, ok bool) {
	if strings.Contains(remoteURL, "://") {
		parsed, err := url.Parse(remoteURL)
		if err != nil {
			return "", "", false
		}
		return parsed.Hostname(), parsed.Path, true
	}
	host, repoPath, found := strings.Cut(remoteURL, ":")
	if !found {
		return "", "", false
	}
	if _, hostname, hasUser := strings.Cut(host, "@"); hasUser {
		host = hostname
	}
	return host, repoPath, true
}

// defaultRepoAliasFromRemote shortens GitHub origins while preserving other origins.
func defaultRepoAliasFromRemote(remoteURL string) string {
	host, repoPath, ok := remoteHostAndPath(remoteURL)
	if !ok {
		return remoteURL
	}
	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "ssh.github.com") {
		return remoteURL
	}
	repoPath = strings.TrimSuffix(strings.Trim(repoPath, "/"), bareRepoSuffix)
	owner, repo, found := strings.Cut(repoPath, "/")
	if !found || isEmpty(owner) || isEmpty(repo) || strings.Contains(repo, "/") {
		return remoteURL
	}
	return repoPath
}

// defaultRepoNameFromRemote derives a structured repository name from a
// remote URL. Hosted paths preserve their grouping (e.g. GitHub
// "nnutter/timber", GitLab "group/sub/repo"); local filesystem paths fall
// back to their basename.
func defaultRepoNameFromRemote(remoteURL string) (string, error) {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "~") {
		return localRepoName(trimmed, remoteURL)
	}
	repoPath, err := hostedRepoPath(trimmed, remoteURL)
	if err != nil {
		return "", err
	}
	return cleanRepoName(repoPath, remoteURL)
}

func localRepoName(trimmed, remoteURL string) (string, error) {
	name := strings.TrimSuffix(path.Base(strings.TrimSuffix(trimmed, "/")), bareRepoSuffix)
	if isReservedPathName(name) {
		return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
	}
	return name, nil
}

func hostedRepoPath(trimmed, remoteURL string) (string, error) {
	switch {
	case strings.HasPrefix(trimmed, "git@"):
		_, remainder, found := strings.Cut(trimmed, ":")
		if !found {
			return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
		}
		return remainder, nil
	case strings.Contains(trimmed, "://"):
		parsed, err := url.Parse(trimmed)
		if err != nil || isEmpty(parsed.Path) {
			return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
		}
		return parsed.Path, nil
	default:
		return scpRepoPath(trimmed), nil
	}
}

func scpRepoPath(trimmed string) string {
	if _, remainder, found := strings.Cut(trimmed, ":"); found {
		return remainder
	}
	return trimmed
}

func cleanRepoName(repoPath, remoteURL string) (string, error) {
	name := strings.TrimSuffix(strings.Trim(repoPath, "/"), bareRepoSuffix)
	name = strings.Trim(name, "/")
	if isReservedPathName(name) {
		return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
	}
	return name, nil
}
