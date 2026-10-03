package timber

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// todoProgress counts checklist items in a worktree's TODO.md.
type todoProgress struct {
	Done  int
	Total int
}

func (x todoProgress) String() string {
	return fmt.Sprintf("%d/%d", x.Done, x.Total)
}

// readTodoProgress counts the `- [ ]` and `- [x]` checklist items in a
// worktree's TODO.md. A missing TODO.md is an empty checklist, not an error,
// because `timber todo` creates the file lazily.
func readTodoProgress(gitDir string) (todoProgress, error) {
	file, err := os.Open(todoFilePath(gitDir))
	if errors.Is(err, fs.ErrNotExist) {
		return todoProgress{}, nil
	}
	if err != nil {
		return todoProgress{}, fmt.Errorf("open %s: %w", todoFilePath(gitDir), err)
	}
	progress, parseErr := parseTodoProgress(file)
	// A close failure on a read-only file carries no information.
	if err := file.Close(); err != nil && parseErr == nil {
		parseErr = fmt.Errorf("close %s: %w", todoFilePath(gitDir), err)
	}
	return progress, parseErr
}

// parseTodoProgress counts checklist items line by line. Items may be nested,
// so leading whitespace is ignored, and only lines whose first non-space
// characters form a checkbox count toward the totals.
func parseTodoProgress(reader io.Reader) (todoProgress, error) {
	var progress todoProgress
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		checked, ok := todoItemChecked(strings.TrimSpace(scanner.Text()))
		if !ok {
			continue
		}
		progress.Total++
		if checked {
			progress.Done++
		}
	}
	if err := scanner.Err(); err != nil {
		return todoProgress{}, fmt.Errorf("read TODO.md: %w", err)
	}
	return progress, nil
}

// todoItemChecked reports whether a trimmed line is a Markdown checkbox and
// whether that checkbox is checked.
func todoItemChecked(line string) (checked bool, ok bool) {
	item, found := strings.CutPrefix(line, "- ")
	if !found {
		return false, false
	}
	switch {
	case strings.HasPrefix(item, "[ ]"):
		return false, true
	case strings.HasPrefix(item, "[x]"), strings.HasPrefix(item, "[X]"):
		return true, true
	}
	return false, false
}
