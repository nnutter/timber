package timber

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// CreateReporters reports worktree creation, Herdr opening, and setup warnings.
// SetupWarning is optional; its errors are ignored.
type CreateReporters struct {
	Created      func(io.Writer, string) error
	HerdrOpened  func(io.Writer, string) error
	SetupPrompt  SetupPrompter
	SetupWarning func(io.Writer, SetupKind, error) error
}

func rejectNonEmptyWorktreeDirectory(worktreePath string) error {
	info, err := os.Stat(worktreePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cannot create worktree directory %q: a file already exists at that path", worktreePath)
	}
	entries, err := os.ReadDir(worktreePath)
	if err != nil {
		return fmt.Errorf("read worktree directory %q: %w", worktreePath, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("worktree directory %q already exists", worktreePath)
	}
	return nil
}
