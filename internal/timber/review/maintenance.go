package review

import (
	"context"
	"time"
)

// startCacheMaintenance prunes the shared patch cache while a review is open.
// The returned function cancels the work and waits for it to stop.
func startCacheMaintenance(ctx context.Context, r Repository) func() {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := r.PruneCache(ctx); err != nil {
			if ctx.Err() == nil {
				r.logger().Warn("Patch cache maintenance failed", "error", err)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
