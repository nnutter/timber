package timber

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// listSort implements pflag.Value for the list command's sort modes.
type listSort string

const (
	listSortRecency  listSort = "recency"
	listSortRepo     listSort = "repo"
	listSortWorktree listSort = "worktree"
)

func (x *listSort) Set(value string) error {
	switch listSort(value) {
	case listSortRecency, listSortRepo, listSortWorktree:
		*x = listSort(value)
		return nil
	default:
		return fmt.Errorf("invalid sort mode %q: expected recency, repo, or worktree", value)
	}
}

func (x *listSort) String() string { return string(*x) }
func (x *listSort) Type() string   { return "sort" }

func completeListSort(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	var values []string
	for _, mode := range []listSort{listSortRecency, listSortRepo, listSortWorktree} {
		if strings.HasPrefix(string(mode), prefix) {
			values = append(values, string(mode))
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}

// Read the recorded HEAD from the bare repository, not the checkout: even a
// missing worktree can be ordered. An unborn HEAD has no commit timestamp.
func (x *listCommandOptions) enrichWorktreeForSort(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	if x.sortBy == listSortRecency && strings.Trim(worktree.CommitHash, "0") != "" {
		result, err := repository.git("show", "-s", "--format=%ct", worktree.CommitHash, "--")
		if err != nil {
			return managedWorktree{}, fmt.Errorf("read commit timestamp for %s@%s: %w", worktree.Name, worktree.Repo, err)
		}
		seconds, err := strconv.ParseInt(strings.TrimSpace(result.stdout), 10, 64)
		if err != nil {
			return managedWorktree{}, fmt.Errorf("parse commit timestamp for %s@%s: %w", worktree.Name, worktree.Repo, err)
		}
		worktree.CommitTime = time.Unix(seconds, 0)
	}
	return x.runtime.enrichWorktreeForList(repository, worktree)
}

func (x listSort) sort(worktrees []managedWorktree) {
	slices.SortFunc(worktrees, func(left, right managedWorktree) int {
		if x == listSortRecency {
			if order := right.CommitTime.Compare(left.CommitTime); order != 0 {
				return order
			}
		}
		if x == listSortWorktree {
			if order := cmp.Compare(left.Name, right.Name); order != 0 {
				return order
			}
		}
		return compareManagedWorktrees(left, right)
	})
}
