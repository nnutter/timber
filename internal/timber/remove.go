package timber

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nnutter/timber/internal/display"
)

// RemoveWorktree resolves a worktree selector and removes it. It returns the
// data the CLI reports and avoids Timber command types in its signature.
func (runtime Runtime) RemoveWorktree(input io.Reader, raw string, force bool) (string, string, string, bool, error) {
	selection := repoSelection{runtime: runtime}
	_, repository, worktree, err := selection.resolveQualifiedWorktree(input, raw)
	if err != nil {
		return "", "", "", false, err
	}
	result, err := runtime.removeResolvedWorktree(repository, worktree, force)
	if err != nil {
		return "", "", "", false, err
	}
	return result.worktree.Name, result.worktree.shortCommitHash(), result.worktree.Path, result.removingCurrentDirectory, nil
}

type worktreeRemovalResult struct {
	worktree                 managedWorktree
	removingCurrentDirectory bool
}

func (runtime Runtime) removeResolvedWorktree(repository *Repository, worktree managedWorktree, force bool) (worktreeRemovalResult, error) {
	name := worktree.Name
	standardLayout, err := samePath(worktree.Path, runtime.managedWorktreePath(worktree.Repo, worktree.branchName()))
	if err != nil {
		return worktreeRemovalResult{}, err
	}

	if !force {
		if _, err := repository.git("fetch", remoteName); err != nil {
			return worktreeRemovalResult{}, fmt.Errorf("fetch %s: %w", remoteName, err)
		}
	}

	if !force {
		worktree, err = runtime.enrichManagedWorktree(repository, worktree)
		if err != nil {
			return worktreeRemovalResult{}, err
		}
		if !worktree.Clean {
			return worktreeRemovalResult{}, fmt.Errorf("worktree %q is not clean", name)
		}
		if worktree.UpstreamRef == "" {
			return worktreeRemovalResult{}, fmt.Errorf("branch %q has no upstream branch; use --force to remove it", worktree.branchName())
		}
		if !worktree.Merged {
			return worktreeRemovalResult{}, fmt.Errorf("branch %q is not merged to %s", worktree.branchName(), shortReference(worktree.UpstreamRef))
		}
	}

	currentDirectory := runtime.CurrentDirectory
	removingCurrentDirectory := currentDirectory != "" && pathIsWithin(worktree.Path, currentDirectory)

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return worktreeRemovalResult{}, err
	}
	if standardLayout {
		if err := removeEmptyParents(worktree.Path, runtime.HomeDirectory); err != nil {
			return worktreeRemovalResult{}, err
		}
	}

	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return worktreeRemovalResult{}, err
	}
	if branchExists {
		if _, err := repository.git("branch", branchDeleteFlag(force), worktree.branchName()); err != nil {
			return worktreeRemovalResult{}, err
		}
	}

	return worktreeRemovalResult{worktree: worktree, removingCurrentDirectory: removingCurrentDirectory}, nil
}

func reportWorktreeRemoval(output io.Writer, worktree managedWorktree, removingCurrentDirectory bool) error {
	if _, err := os.Stat(worktree.Path); err == nil {
		if _, writeErr := fmt.Fprintf(output, "%s\n", display.WarningStyle.Render("warning: worktree directory still exists: "+worktree.Path)); writeErr != nil {
			return writeErr
		}
	}

	message := fmt.Sprintf("removed %s at %s", worktree.Name, worktree.shortCommitHash())
	if _, err := fmt.Fprintf(output, "%s\n", display.StatusStyle.Render(message)); err != nil {
		return err
	}
	if removingCurrentDirectory {
		_, err := fmt.Fprintf(output, "%s\n", display.WarningStyle.Render("current directory was removed"))
		return err
	}
	return nil
}

// removeEmptyParents removes path and empty ancestor directories up to (but not
// including) stopPath or the filesystem root.
func removeEmptyParents(path string, stopPath string) error {
	current := canonicalPath(path)
	stopPath = canonicalPath(stopPath)

	for {
		if current == stopPath || current == string(filepath.Separator) {
			return nil
		}
		if !pathIsWithin(stopPath, current) {
			return nil
		}

		err := os.Remove(current)
		if err == nil {
			current = filepath.Dir(current)
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			current = filepath.Dir(current)
			continue
		}
		if isNotEmptyError(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", current, err)
	}
}
