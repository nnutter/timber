package timber

import (
	"context"
	"io"
)

// ListWorktrees lists managed worktrees as a table or JSON document. All
// output is written to the given writers, preserving CLI reporting order.
func (runtime Runtime) ListWorktrees(ctx context.Context, stdout io.Writer, repoArg string, pullRequests, location, jsonOutput bool, sortMode string) error {
	var sortBy listSort
	if err := sortBy.Set(sortMode); err != nil {
		return err
	}

	selection := repoSelection{runtime: runtime}
	if repoArg != "" {
		repo, err := runtime.parseRepoOnlyArg(repoArg)
		if err != nil {
			return err
		}
		selection.RepoName = repo
	}

	repos, err := selection.reposToConsider()
	if err != nil {
		return err
	}
	enrich := func(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
		return enrichWorktreeForSort(runtime, repository, worktree, sortBy)
	}
	worktrees, err := runtime.collectWorktrees(repos, enrich)
	if err != nil {
		return err
	}
	sortBy.sort(worktrees)
	runtime.enrichListedWorktreesWithReviews(ctx, worktrees)
	if pullRequests {
		if err := runtime.enrichListedWorktreesWithPullRequests(ctx, worktrees); err != nil {
			return err
		}
	}

	if jsonOutput {
		return reportListJSON(stdout, worktrees)
	}
	return runtime.reportListTable(stdout, worktrees, pullRequests, location)
}
