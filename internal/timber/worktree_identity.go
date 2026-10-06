package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const worktreeNameFile = "timber-name"

// Standard-layout worktrees predate explicit name records. An empty name
// identifies a manual checkout whose name must be derived from its path.
func (x Runtime) recordedWorktreeName(repoName, path, branch string) (string, error) {
	same, err := samePath(path, x.managedWorktreePath(repoName, branch))
	if err != nil {
		return "", err
	}
	if same {
		return branch, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	gitDir, err := x.worktreeGitDir(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(gitDir, worktreeNameFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read worktree name: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

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
