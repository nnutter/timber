package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AddRepo registers a bare repository from a remote URL or path. It
// returns the repository name and target path for CLI reporting.
func (runtime Runtime) AddRepo(source, name, alias string) (string, string, error) {
	remoteURL, err := resolveRemoteURL(source)
	if err != nil {
		return "", "", err
	}

	repoName := normalizeRepoName(name)
	if repoName == "" {
		repoName, err = defaultRepoNameFromRemote(remoteURL)
		if err != nil {
			return "", "", err
		}
	}
	if err := validateRepoName(repoName); err != nil {
		return "", "", err
	}

	targetPath := runtime.bareRepoPath(repoName)
	if _, err := os.Stat(targetPath); err == nil {
		return "", "", fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}

	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return "", "", err
	}

	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "clone", "--bare", remoteURL, targetPath); err != nil {
		return "", "", err
	}

	if err := configureBareOriginTracking(runtime, targetPath); err != nil {
		return "", "", err
	}

	if alias != "" {
		if _, err := gitOutput(runtime, targetPath, "config", "--local", "timber.alias", alias); err != nil {
			return "", "", err
		}
	}

	return repoName, targetPath, nil
}
