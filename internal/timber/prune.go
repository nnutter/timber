package timber

import (
	"io"

	"github.com/samber/lo"
)

// PruneWorktrees removes merged and clean managed worktrees. With prompt it
// asks which worktrees to remove; with dryRun it only reports candidates.
// All progress is written to output, preserving CLI reporting order.
func (runtime Runtime) PruneWorktrees(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool, prompter WorktreePrompter) error {
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

	enrichedWorktrees, err := runtime.collectManagedWorktrees(repos)
	if err != nil {
		return err
	}

	var selectedWorktrees []managedWorktree
	if prompt {
		if prompter == nil {
			prompter = huhWorktreePrompter{}
		}
		refs := make([]WorktreeRef, 0, len(enrichedWorktrees))
		for _, worktree := range enrichedWorktrees {
			refs = append(refs, worktree.ref())
		}
		selectedRefs, err := prompter.Prompt(input, output, refs)
		if err != nil {
			return err
		}
		for _, ref := range selectedRefs {
			worktree, err := managedWorktreeByKey(enrichedWorktrees, worktreeRefKey(ref))
			if err != nil {
				return err
			}
			selectedWorktrees = append(selectedWorktrees, worktree)
		}
	} else {
		selectedWorktrees = lo.Filter(enrichedWorktrees, func(worktree managedWorktree, _ int) bool {
			return worktree.Clean && worktree.Merged
		})
	}

	if dryRun {
		return runtime.reportPruneDryRun(output, selectedWorktrees)
	}

	for _, worktree := range selectedWorktrees {
		if !prompt && (!worktree.Clean || !worktree.Merged) {
			continue
		}
		// Removing a checkout can shorten other directory-derived names.
		// Use the selected identity rather than resolving its name again.
		repository, _, err := runtime.openRegisteredRepository(worktree.Repo)
		if err != nil {
			return err
		}
		result, err := runtime.removeResolvedWorktree(repository, worktree, true)
		if err != nil {
			return err
		}
		if err := reportWorktreeRemoval(output, result.worktree, result.removingCurrentDirectory); err != nil {
			return err
		}
	}

	return nil
}
