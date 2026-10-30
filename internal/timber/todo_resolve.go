package timber

import (
	"fmt"
	"io"
	"path/filepath"
)

func (x Runtime) ResolveTodoRepoGitDir(input io.Reader, args []string) (string, error) {
	gitDir, err := x.ResolveTodoGitDir(input, args)
	if err != nil {
		return "", err
	}
	result, err := gitOutput(x, gitDir, "--git-dir", gitDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolve common git dir: %w", err)
	}
	return filepath.Clean(result.stdout), nil
}

func (x Runtime) ResolveTodoGitDir(input io.Reader, args []string) (string, error) {
	if len(args) == 0 {
		gitDir, err := gitOutput(x, x.CurrentDirectory, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
		}
		bare, err := gitOutput(x, x.CurrentDirectory, "rev-parse", "--is-bare-repository")
		if err != nil {
			return "", err
		}
		if bare.stdout == "true" {
			return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
		}
		return filepath.Clean(gitDir.stdout), nil
	}
	selection := repoSelection{runtime: x}
	_, _, worktree, err := selection.resolveQualifiedWorktree(input, args[0])
	if err != nil {
		return "", err
	}
	repository, err := openRepository(x, worktree.Path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(repository.GitDir), nil
}
