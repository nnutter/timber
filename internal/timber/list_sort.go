package timber

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// listSort implements pflag.Value for the list command's sort modes.
type listSort string

const (
	listSortRepo     listSort = "repo"
	listSortWorktree listSort = "worktree"
)

func (x *listSort) Set(value string) error {
	switch listSort(value) {
	case listSortRepo, listSortWorktree:
		*x = listSort(value)
		return nil
	default:
		return fmt.Errorf("invalid sort mode %q: expected repo or worktree", value)
	}
}

func (x *listSort) String() string { return string(*x) }
func (x *listSort) Type() string   { return "sort" }

func completeListSort(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	var values []string
	for _, mode := range []listSort{listSortRepo, listSortWorktree} {
		if strings.HasPrefix(string(mode), prefix) {
			values = append(values, string(mode))
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}

func (x listSort) sort(worktrees []managedWorktree) {
	slices.SortFunc(worktrees, func(left, right managedWorktree) int {
		if x == listSortWorktree {
			if order := cmp.Compare(left.Name, right.Name); order != 0 {
				return order
			}
		}
		return compareManagedWorktrees(left, right)
	})
}
