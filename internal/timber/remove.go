package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type worktreeRemovalResult struct {
	worktree                 managedWorktree
	removingCurrentDirectory bool
}

// removeEmptyParents removes path and empty ancestor directories up to (but not
// including) stopPath or the filesystem root.
func removeEmptyParents(path string, stopPath string) error {
	current := canonicalPath(path)
	stopPath = canonicalPath(stopPath)

	for {
		if current == stopPath || isSame(current, string(filepath.Separator)) {
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
