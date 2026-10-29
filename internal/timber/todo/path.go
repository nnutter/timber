package todo

import "path/filepath"

// Path returns the TODO.md location for a worktree's Git directory.
func Path(gitDir string) string {
	return filepath.Join(gitDir, "TODO.md")
}
