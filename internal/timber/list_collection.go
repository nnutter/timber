package timber

func collectListedWorktrees(runtime Runtime, repos []registeredRepo, enrich worktreeEnricher) ([]managedWorktree, error) {
	worktrees := make([]managedWorktree, 0)
	for _, repo := range repos {
		listed, err := listRepositoryWorktrees(runtime, repo, enrich)
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, listed...)
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
