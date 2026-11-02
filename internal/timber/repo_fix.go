package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const legacyReposDirName = "timber/repos"

func worktreeGitDirIsStale(worktreePath string, barePath string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read worktree gitdir %q: %w", worktreePath, err)
	}
	line := strings.TrimSpace(string(content))
	gitdir, ok := strings.CutPrefix(line, "gitdir: ")
	if !ok {
		return true, nil
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktreePath, gitdir)
	}
	return !pathIsWithin(barePath, filepath.Clean(gitdir)), nil
}
