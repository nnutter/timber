package timber

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

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

func githubRemoteOwner(remote string) string {
	var host, repoPath string
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil || !isSupportedRemoteScheme(parsed.Scheme) {
			return ""
		}
		host, repoPath = parsed.Hostname(), parsed.Path
		if strings.EqualFold(host, "ssh.github.com") && !isSame(parsed.Scheme, "ssh") {
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
	if !found || !githubLoginPattern.MatchString(owner) || isReservedRepoName(repo) || strings.Contains(repo, "/") {
		return ""
	}
	return owner
}
