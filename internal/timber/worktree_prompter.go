package timber

import (
	"fmt"
	"io"
	"slices"
)

// WorktreeRef identifies a managed worktree for selection UI. It carries
// the display fields prompts need without exposing worktree internals.
type WorktreeRef struct {
	Repo        string
	Name        string
	Path        string
	DisplayPath string
	Status      string
	Clean       bool
	Merged      bool
}

// WorktreePrompter selects worktrees interactively.
type WorktreePrompter interface {
	Prompt(io.Reader, io.Writer, []WorktreeRef) ([]WorktreeRef, error)
}

func worktreeRefKey(ref WorktreeRef) string {
	return ref.Repo + "\x00" + ref.Name
}

func pruneWorktreeKey(worktree managedWorktree) string {
	return worktree.Repo + "\x00" + worktree.Name
}

func managedWorktreeByKey(worktrees []managedWorktree, key string) (managedWorktree, error) {
	index := slices.IndexFunc(worktrees, func(worktree managedWorktree) bool {
		return pruneWorktreeKey(worktree) == key
	})
	if index >= 0 {
		return worktrees[index], nil
	}
	return managedWorktree{}, fmt.Errorf("unknown worktree %q", key)
}
