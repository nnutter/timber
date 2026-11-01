package timber

import (
	"path/filepath"
)

const worktreeNameFile = "timber-name"

// Expand all conflicting manual names together, reserving explicit names.
// Distinct absolute paths eventually yield distinct names, even when one
// checkout's path is a suffix of another's.
func assignManualWorktreeNames(worktrees []managedWorktree) {
	manual := make([]bool, len(worktrees))
	parents := make([]string, len(worktrees))
	for index := range worktrees {
		if worktrees[index].Name == "" {
			manual[index] = true
			worktrees[index].Name = filepath.Base(worktrees[index].Path)
			parents[index] = filepath.Dir(worktrees[index].Path)
		}
	}
	for {
		counts := make(map[string]int)
		for _, worktree := range worktrees {
			counts[worktree.Name]++
		}
		changed := false
		for index := range worktrees {
			if !manual[index] || counts[worktrees[index].Name] < 2 {
				continue
			}
			parent := parents[index]
			if parent == string(filepath.Separator) {
				worktrees[index].Name = "/" + worktrees[index].Name
			} else {
				worktrees[index].Name = filepath.Base(parent) + "/" + worktrees[index].Name
			}
			parents[index] = filepath.Dir(parent)
			changed = true
		}
		if !changed {
			return
		}
	}
}
