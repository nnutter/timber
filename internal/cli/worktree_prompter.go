package cli

import (
	"fmt"
	"io"
	"slices"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"

	"github.com/nnutter/timber/internal/timber"
)

func worktreeRefKey(ref timber.WorktreeRef) string {
	return ref.Repo + "\x00" + ref.Name
}

type huhWorktreePrompter struct{}

func (huhWorktreePrompter) Prompt(input io.Reader, output io.Writer, worktrees []timber.WorktreeRef) ([]timber.WorktreeRef, error) {
	selectedKeys := make([]string, 0)
	options := lo.Map(worktrees, func(worktree timber.WorktreeRef, _ int) huh.Option[string] {
		label := fmt.Sprintf("%s %s (%s) %s", worktree.Repo, worktree.Name, worktree.DisplayPath, worktree.Status)
		if worktree.Clean {
			label += " (clean)"
		} else {
			label += " (dirty)"
		}
		option := huh.NewOption(label, worktreeRefKey(worktree))
		if worktree.Clean && isTrue(worktree.Merged) {
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

	selectedWorktrees := make([]timber.WorktreeRef, 0, len(selectedKeys))
	for _, selectedKey := range selectedKeys {
		worktree, err := worktreeRefByKey(worktrees, selectedKey)
		if err != nil {
			return nil, err
		}
		selectedWorktrees = append(selectedWorktrees, worktree)
	}

	return selectedWorktrees, nil
}

func worktreeRefByKey(worktrees []timber.WorktreeRef, key string) (timber.WorktreeRef, error) {
	index := slices.IndexFunc(worktrees, func(worktree timber.WorktreeRef) bool {
		return worktreeRefKey(worktree) == key
	})
	if index >= 0 {
		return worktrees[index], nil
	}
	return timber.WorktreeRef{}, fmt.Errorf("unknown worktree %q", key)
}
