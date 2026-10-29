package timber

import (
	"fmt"
	"os"
	"strings"
)

func (runtime Runtime) removeRepo(repoName string) error {
	repository, repo, err := runtime.openRegisteredRepository(repoName)
	if err != nil {
		return err
	}

	worktrees, err := runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}
	if len(worktrees) > 0 {
		names := make([]string, 0, len(worktrees))
		for _, worktree := range worktrees {
			names = append(names, worktree.Name)
		}
		return fmt.Errorf(
			"repository %q still has managed worktrees (%s); remove them first",
			repoName,
			strings.Join(names, ", "),
		)
	}

	// Also refuse if git still has any non-bare linked worktrees (unmanaged).
	porcelain, err := repository.listPorcelainWorktrees()
	if err != nil {
		return err
	}
	linked := 0
	for _, worktree := range porcelain {
		if worktree.Bare {
			continue
		}
		same, err := samePath(worktree.Path, repo.BarePath)
		if err != nil {
			return err
		}
		if same {
			continue
		}
		linked++
	}
	if linked > 0 {
		return fmt.Errorf("repository %q still has %d linked worktree(s); remove them first", repoName, linked)
	}

	if err := os.RemoveAll(repo.BarePath); err != nil {
		return fmt.Errorf("remove bare repository %q: %w", repo.BarePath, err)
	}
	if err := runtime.removeEmptyBareParents(repo.BarePath); err != nil {
		return err
	}

	return nil
}
