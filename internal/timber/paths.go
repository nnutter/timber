package timber

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const bareRepoSuffix = ".git"

// normalizeRepoName strips surrounding whitespace and slashes plus a
// trailing ".git" so worktree paths use the clean repo name (e.g. "roam")
// rather than the bare-dir style name ("roam.git"). Structured names keep
// their grouping prefix ("nnutter/timber").
func normalizeRepoName(name string) string {
	trimmed := strings.Trim(strings.TrimSpace(name), "/")
	trimmed = strings.TrimSuffix(trimmed, bareRepoSuffix)
	return strings.Trim(trimmed, "/")
}

// repoShortName returns the final segment of a repository name. Simple names
// return unchanged; structured names (e.g. "nnutter/timber") drop the
// grouping prefix ("timber"). The short name is the leaf directory of a
// managed worktree checkout.
func repoShortName(name string) string {
	if _, short, found := strings.CutLast(name, "/"); found {
		return short
	}
	return name
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
