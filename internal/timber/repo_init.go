package timber

import (
	"os"
	"path/filepath"
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
