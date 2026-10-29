package timber

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const switchPathFileEnvVarName = "TIMBER_SWITCH_PATH_FILE"

// ResolveSwitchWorktree resolves a worktree selector to its name and path.
// alreadyIn reports whether the current directory is already the target.
func (runtime Runtime) ResolveSwitchWorktree(input io.Reader, raw, repoName string) (name, path string, alreadyIn bool, err error) {
	qualified, err := runtime.parseQualifiedName(raw)
	if err != nil {
		return "", "", false, err
	}
	if qualified.Repo != "" {
		repoName = qualified.Repo
	}

	name = qualified.Name
	if name == "" {
		return "", "", false, fmt.Errorf("worktree name is required")
	}
	if repoName == "" {
		repoName, err = runtime.inferUniqueRepoForWorktree(name)
		if err != nil {
			return "", "", false, err
		}
	}

	repository, _, err := runtime.openRegisteredRepository(repoName)
	if err != nil {
		return "", "", false, err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, repoName)
	if err != nil {
		return "", "", false, err
	}
	worktree, err := managedWorktreeByName(worktrees, name)
	if err != nil {
		return "", "", false, fmt.Errorf("worktree %s not found in repository %s", name, repoName)
	}
	path = worktree.Path
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", false, fmt.Errorf("worktree %s not found at %s", name, path)
		}
		return "", "", false, fmt.Errorf("inspect worktree directory %q: %w", path, err)
	}

	same, err := samePath(runtime.CurrentDirectory, path)
	if err != nil {
		return "", "", false, err
	}
	return name, path, same, nil
}
