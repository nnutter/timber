package cli

import (
	"path/filepath"

	"github.com/nnutter/timber/internal/timber"
)

func repoTestBarePath(runtime timber.Runtime, name string) string {
	return filepath.Join(runtime.WorktreeRoot, filepath.FromSlash(name), ".git")
}

func repoTestWorktreePath(runtime timber.Runtime, repo, name string) string {
	return filepath.Join(runtime.WorktreeRoot, filepath.FromSlash(repo), name, filepath.Base(repo))
}
