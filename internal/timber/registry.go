package timber

import (
	"fmt"
	"strings"
)

type registeredRepo struct {
	Name     string
	BarePath string
	GitDir   string // Git's absolute directory, resolved during registration discovery.
}

func (x registeredRepo) alias(runtime Runtime, origin string) string {
	result, err := gitOutput(runtime, x.BarePath, "config", "--local", "--get", "timber.alias")
	if err == nil && !isEmpty(result.stdout) {
		return result.stdout
	}
	return defaultRepoAliasFromRemote(origin)
}

func (x registeredRepo) originURL(runtime Runtime) string {
	result, err := gitOutput(runtime, x.BarePath, "remote", "get-url", remoteName)
	if err != nil {
		return ""
	}
	return result.stdout
}

func (x registeredRepo) ref() RepoRef {
	return RepoRef{Name: x.Name, BarePath: x.BarePath}
}

func registeredBareGitDir(runtime Runtime, path string) (string, error) {
	result, err := gitOutput(runtime, path, "rev-parse", "--absolute-git-dir", "--is-bare-repository")
	if err != nil {
		return "", err
	}
	dir, bare, _ := strings.CutLast(result.stdout, "\n")
	if bare != "true" {
		return "", fmt.Errorf("repository at %q is not bare", path)
	}
	return strings.TrimSpace(dir), nil
}
