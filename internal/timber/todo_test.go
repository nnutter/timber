package timber

import (
	"strings"
	"testing"

	"github.com/nnutter/timber/internal/timber/todo"
)

// Application list tests use this fixture without depending on CLI assembly.
func todoPathForWorktree(t *testing.T, worktreePath string) string {
	t.Helper()

	gitDir := strings.TrimSpace(runGitCommand(t, worktreePath, "rev-parse", "--absolute-git-dir"))
	return todo.Path(gitDir)
}
