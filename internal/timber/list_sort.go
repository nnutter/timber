package timber

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
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

// Read the recorded HEAD from the bare repository, not the checkout: even a
// missing worktree can be ordered. An unborn HEAD has no commit timestamp.
func enrichWorktreeForSort(runtime Runtime, repository *Repository, worktree managedWorktree, sortBy listSort) (managedWorktree, error) {
	if sortBy == listSortRecency && strings.Trim(worktree.CommitHash, "0") != "" {
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
	return runtime.enrichWorktreeForList(repository, worktree)
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
