package timber

import "sync"

const listRepositoryConcurrency = 8

func collectListedWorktrees(runtime Runtime, repos []registeredRepo, enrich worktreeEnricher) ([]managedWorktree, error) {
	type result struct {
		worktrees []managedWorktree
		err       error
	}
	results := make([]result, len(repos))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(listRepositoryConcurrency, len(repos)) {
		workers.Go(func() {
			for index := range jobs {
				results[index].worktrees, results[index].err = listRepositoryWorktrees(runtime, repos[index], enrich)
			}
		})
	}
	for index := range repos {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	// Preserve serial ordering, including which error is reported first.
	worktrees := make([]managedWorktree, 0)
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		worktrees = append(worktrees, result.worktrees...)
	}
	return worktrees, nil
}

func listRepositoryWorktrees(runtime Runtime, repo registeredRepo, enrich worktreeEnricher) ([]managedWorktree, error) {
	repository := new(Repository{GitDir: repo.GitDir, Runtime: runtime})
	worktrees, err := runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return nil, err
	}
	for i, worktree := range worktrees {
		enriched, err := enrich(repository, worktree)
		if err != nil {
			return nil, err
		}
		worktrees[i] = enriched
	}
	return worktrees, nil
}
