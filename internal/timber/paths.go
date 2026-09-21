package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const bareRepoSuffix = ".git"

// normalizeRepoName strips a trailing ".git" so worktree paths use the short
// repo name (e.g. "roam") rather than the bare-dir style name ("roam.git").
func normalizeRepoName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), bareRepoSuffix)
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}

// pathIsWithin reports whether child is the same as parent or nested under it.
func pathIsWithin(parent string, child string) bool {
	parent = canonicalPath(parent)
	child = canonicalPath(child)
	relativePath, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)))
}

func canonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}
