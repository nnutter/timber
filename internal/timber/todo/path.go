package todo

import "path/filepath"

// Path returns the TODO.md location for a Git directory.
func Path(gitDir string) string {
	return filepath.Join(gitDir, "TODO.md")
}

// LegacyRepoPath returns the old repository TODO filename for migration.
func LegacyRepoPath(gitDir string) string {
	return filepath.Join(gitDir, "TODO.repo.md")
}
