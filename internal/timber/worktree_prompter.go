package timber

import (
	"fmt"
	"io"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
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

func (x managedWorktree) ref() WorktreeRef {
	return WorktreeRef{
		Repo:        x.Repo,
		Name:        x.Name,
		Path:        x.Path,
		DisplayPath: x.DisplayPath,
		Status:      x.Status,
		Clean:       x.Clean,
		Merged:      x.Merged,
	}
}

func worktreeRefKey(ref WorktreeRef) string {
	return ref.Repo + "\x00" + ref.Name
}

type huhWorktreePrompter struct{}

func (huhWorktreePrompter) Prompt(input io.Reader, output io.Writer, worktrees []WorktreeRef) ([]WorktreeRef, error) {
	selectedKeys := make([]string, 0)
	options := lo.Map(worktrees, func(worktree WorktreeRef, _ int) huh.Option[string] {
		label := fmt.Sprintf("%s %s (%s) %s", worktree.Repo, worktree.Name, worktree.DisplayPath, worktree.Status)
		if worktree.Clean {
			label += " (clean)"
		} else {
			label += " (dirty)"
		}
		option := huh.NewOption(label, worktreeRefKey(worktree))
		if worktree.Clean && worktree.Merged {
			option = option.Selected(true)
		}
		return option
	})

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select worktrees to prune").
				Options(options...).
				Value(&selectedKeys),
		),
	).WithInput(input).WithOutput(output)

	if err := form.Run(); err != nil {
		return nil, err
	}

	selectedWorktrees := make([]WorktreeRef, 0, len(selectedKeys))
	for _, selectedKey := range selectedKeys {
		worktree, err := worktreeRefByKey(worktrees, selectedKey)
		if err != nil {
			return nil, err
		}
		selectedWorktrees = append(selectedWorktrees, worktree)
	}

	return selectedWorktrees, nil
}

func worktreeRefByKey(worktrees []WorktreeRef, key string) (WorktreeRef, error) {
	index := slices.IndexFunc(worktrees, func(worktree WorktreeRef) bool {
		return worktreeRefKey(worktree) == key
	})
	if index >= 0 {
		return worktrees[index], nil
	}
	return WorktreeRef{}, fmt.Errorf("unknown worktree %q", key)
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
