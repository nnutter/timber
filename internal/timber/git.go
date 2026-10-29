package timber

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ResolveGitSelector interprets a candidate as a <worktree>, <worktree>@<repo>,
// or @<repo> selector. A candidate containing @ always selects a target and
// returns its errors. A bare worktree name selects the managed worktree, while
// a repo-only @<repo> selects the bare repository. A bare name selects a
// worktree only when it resolves to an existing managed worktree; otherwise
// selected is false and the caller can treat the candidate as a Git subcommand so
// commands such as `status` keep working. The returned gitDir is set for the repo-only
// case; it is empty for worktrees so the caller resolves it from the
// worktree directory.
func (runtime Runtime) ResolveGitSelector(input io.Reader, candidate string) (string, string, bool, error) {
	x := repoSelection{runtime: runtime}
	if strings.Contains(candidate, "@") {
		qualified, err := x.runtime.parseQualifiedName(candidate)
		if err != nil {
			return "", "", false, err
		}
		if qualified.Name == "" {
			repo, err := x.runtime.registeredRepoByName(qualified.Repo)
			if err != nil {
				return "", "", false, err
			}
			return repo.BarePath, repo.BarePath, true, nil
		}
		_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
		if err != nil {
			return "", "", false, err
		}
		return worktree.Path, "", true, nil
	}

	repoName, err := x.runtime.inferUniqueRepoForWorktree(candidate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	x.RepoName = repoName
	_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
	if err != nil {
		return "", "", false, err
	}
	return worktree.Path, "", true, nil
}

// GitDirForDirectory resolves a worktree Git directory and rejects bare repositories.
func (runtime Runtime) GitDirForDirectory(directory string) (string, error) {
	gitDirResult, err := gitOutput(runtime, directory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or @<repo> or run inside a worktree")
	}

	bareResult, err := gitOutput(runtime, directory, "rev-parse", "--is-bare-repository")
	if err != nil {
		return "", err
	}
	if bareResult.stdout == "true" {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or @<repo> or run inside a worktree")
	}
	return filepath.Clean(gitDirResult.stdout), nil
}
