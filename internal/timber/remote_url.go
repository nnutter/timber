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
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
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
// https://github.com/<owner>/<repo> without a .git suffix. Absolute URLs,
// SSH forms, and local paths pass through unchanged.
func resolveRemoteURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("repository URL is required")
	}

	switch {
	case strings.Contains(trimmed, "://"):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "git@"):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "github.com:"):
		return "git@" + trimmed, nil
	case strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "."):
		return trimmed, nil
	case strings.HasPrefix(trimmed, "~"):
		return trimmed, nil
	case looksLikeSSHShorthand(trimmed):
		return trimmed, nil
	default:
		if short, ok := parseGitHubShortForm(trimmed); ok {
			return "https://github.com/" + short, nil
		}
		return "https://github.com/" + strings.TrimPrefix(trimmed, "/"), nil
	}
}

func looksLikeSSHShorthand(input string) bool {
	// host:path without scheme, e.g. gitlab.com:group/repo.git
	if strings.Contains(input, "://") {
		return false
	}
	host, repoPath, found := strings.Cut(input, ":")
	if !found || host == "" || repoPath == "" {
		return false
	}
	return !strings.Contains(host, "/") && strings.Contains(host, ".")
}

// defaultRepoAliasFromRemote shortens GitHub origins while preserving other origins.
func defaultRepoAliasFromRemote(remoteURL string) string {
	var host, repoPath string
	if strings.Contains(remoteURL, "://") {
		parsed, err := url.Parse(remoteURL)
		if err != nil {
			return remoteURL
		}
		host, repoPath = parsed.Hostname(), parsed.Path
	} else {
		host, repoPath, _ = strings.Cut(remoteURL, ":")
		if _, hostname, found := strings.Cut(host, "@"); found {
			host = hostname
		}
	}
	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "ssh.github.com") {
		return remoteURL
	}
	repoPath = strings.TrimSuffix(strings.Trim(repoPath, "/"), bareRepoSuffix)
	owner, repo, found := strings.Cut(repoPath, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
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
		name := strings.TrimSuffix(path.Base(strings.TrimSuffix(trimmed, "/")), bareRepoSuffix)
		if name == "" || name == "." || name == "/" {
			return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
		}
		return name, nil
	}

	var repoPath string
	switch {
	case strings.HasPrefix(trimmed, "git@"):
		_, remainder, found := strings.Cut(trimmed, ":")
		if !found {
			return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
		}
		repoPath = remainder
	case strings.Contains(trimmed, "://"):
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Path == "" {
			return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
		}
		repoPath = parsed.Path
	default:
		if _, remainder, found := strings.Cut(trimmed, ":"); found {
			repoPath = remainder
		} else {
			repoPath = trimmed
		}
	}

	name := strings.TrimSuffix(strings.Trim(repoPath, "/"), bareRepoSuffix)
	name = strings.Trim(name, "/")
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("could not derive repository name from %q", remoteURL)
	}
	return name, nil
}
