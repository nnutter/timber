package timber

import (
	"errors"
	"io"

	"github.com/samber/lo"
)

// PruneReporters reports dry-run candidates and completed removals.
type PruneReporters struct {
	DryRun  func(io.Writer, string, string, string) error
	Removal func(io.Writer, string, string, string, bool) error
}

// PruneWorktrees removes merged and clean managed worktrees. With prompt it
// asks which worktrees to remove; with dryRun it only reports candidates.
// A non-nil prompter is required when prompt is true.
// All progress is written to output, preserving CLI reporting order.
func (runtime Runtime) PruneWorktrees(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool, prompter WorktreePrompter, reporters PruneReporters) error {
	if prompt && prompter == nil {
		return errors.New("prune prompt requires a worktree prompter")
	}
	if dryRun && reporters.DryRun == nil {
		return errors.New("prune dry run requires a reporter")
	}
	if !dryRun && reporters.Removal == nil {
		return errors.New("prune removal requires a reporter")
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

	enrichedWorktrees, err := runtime.collectManagedWorktrees(repos)
	if err != nil {
		return err
	}

	var selectedWorktrees []managedWorktree
	if prompt && prompter != nil {
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
		for _, worktree := range selectedWorktrees {
			if err := reporters.DryRun(output, worktree.Name, worktree.Repo, runtime.displayHomePath(worktree.Path)); err != nil {
				return err
			}
		}
		return nil
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
		if err := reporters.Removal(output, result.worktree.Name, result.worktree.shortCommitHash(), result.worktree.Path, result.removingCurrentDirectory); err != nil {
			return err
		}
	}

	return nil
}
