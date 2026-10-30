package timber

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// defaultInitialBranch is the first branch of a repository created by
	// InitRepo. Git's own fallback depends on configuration and version, so
	// name the branch to keep the managed layout predictable.
	defaultInitialBranch = "main"

	// initialCommitMessage is the subject of the empty commit that gives a new
	// repository a branch for worktrees to attach to.
	initialCommitMessage = "Initial commit"
)

// InitRepo creates an empty bare repository under the worktree root. It
// returns the repository name and Git directory for CLI reporting.
//
// The repository has no remote. Its initial branch carries one empty commit so
// managed worktrees can branch from it before the first push.
func (runtime Runtime) InitRepo(name, initialBranch string) (string, string, error) {
	repoName := normalizeRepoName(name)
	if err := validateRepoName(repoName); err != nil {
		return "", "", err
	}

	branch := cmp.Or(strings.TrimSpace(initialBranch), defaultInitialBranch)
	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "check-ref-format", "--branch", branch); err != nil {
		return "", "", fmt.Errorf("invalid initial branch %q: %w", branch, err)
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

	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "init", "--bare", targetPath); err != nil {
		return "", "", err
	}

	if err := initializeBranch(runtime, targetPath, branch); err != nil {
		// A repository without its initial commit cannot serve worktrees and
		// would block a retry by existing, so discard the half-made repository.
		removeFailedInit(targetPath, runtime.worktreeRoot())
		return "", "", err
	}

	return repoName, targetPath, nil
}

// initializeBranch points HEAD at the initial branch and records the empty
// commit that the branch starts from.
func initializeBranch(runtime Runtime, barePath, branch string) error {
	repository, err := openBareRepository(runtime, barePath)
	if err != nil {
		return err
	}
	if _, err := repository.git("symbolic-ref", "HEAD", branchRefPrefix+branch); err != nil {
		return err
	}
	return repository.createEmptyCommit(branch)
}

// removeFailedInit deletes the Git directory of a repository whose
// initialization failed. Directories that only that repository used are
// removed up to the worktree root, so a retry on the same path starts clean;
// directories that hold anything else stay in place.
func removeFailedInit(barePath, worktreeRoot string) {
	_ = os.RemoveAll(barePath)
	_ = removeEmptyParents(filepath.Dir(barePath), worktreeRoot)
}

// createEmptyCommit records an empty commit on branch so the branch exists and
// worktrees can attach to it.
func (x *Repository) createEmptyCommit(branchName string) error {
	// mktree reads the tree entries from stdin. No entries means the empty
	// tree, which is the only content an initial commit needs.
	tree, err := x.git("mktree")
	if err != nil {
		return fmt.Errorf("create empty tree: %w", err)
	}
	// commit-tree takes the author and committer from Git configuration, so a
	// missing identity reports Git's own guidance.
	commit, err := x.git("commit-tree", tree.stdout, "-m", initialCommitMessage)
	if err != nil {
		return fmt.Errorf("create initial commit: %w", err)
	}
	if _, err := x.git("update-ref", branchRefPrefix+branchName, commit.stdout); err != nil {
		return fmt.Errorf("record initial commit: %w", err)
	}
	return nil
}
