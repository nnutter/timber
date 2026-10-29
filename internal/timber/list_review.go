package timber

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nnutter/timber/internal/timber/review"
)

const reviewLookupConcurrency = 4

// reviewProgress counts commits, including fixups, rather than review stacks.
// A nil progress means the status could not be read, not zero approvals.
type reviewProgress struct {
	Approved int `json:"approved"`
	Total    int `json:"total"`
}

func (x *reviewProgress) String() string {
	if x == nil {
		return "?"
	}
	return fmt.Sprintf("%d/%d", x.Approved, x.Total)
}

func (x Runtime) enrichListedWorktreesWithReviews(ctx context.Context, worktrees []managedWorktree) {
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(reviewLookupConcurrency, len(worktrees)) {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				worktrees[index].Review = x.worktreeReviewProgress(ctx, worktrees[index].Path)
			}
		})
	}
	for index := range worktrees {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
}

func (x Runtime) worktreeReviewProgress(ctx context.Context, path string) *reviewProgress {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	service := review.Service{Repository: review.Repository{Dir: path, Environment: x.Environment}}
	result, err := service.Status(ctx, false)
	if err != nil || result.Version != 1 || result.Command != "status" {
		return nil
	}
	progress := new(reviewProgress)
	for _, entry := range result.Summary {
		progress.Total += entry.Count
		if entry.State == "approved" {
			progress.Approved = entry.Count
		}
	}
	return progress
}
