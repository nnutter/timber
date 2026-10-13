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

// todoEntry is one Markdown checkbox in TODO.md. ID is the 1-based line
// number in the file, and Indent is the leading-whitespace width used to
// resolve parent-child relationships.
type todoEntry struct {
	ID       int
	Indent   int
	Title    string
	Complete bool
}

// parseTodoProgress counts checklist items line by line. Items may be nested,
// so leading whitespace is ignored, and only lines whose first non-space
// characters form a checkbox count toward the totals.
func parseTodoProgress(reader io.Reader) (todoProgress, error) {
	entries, err := parseTodoEntries(reader)
	if err != nil {
		return todoProgress{}, err
	}
	var progress todoProgress
	progress.Total = len(entries)
	for _, entry := range entries {
		if entry.Complete {
			progress.Done++
		}
	}
	return progress, nil
}

// parseTodoEntries returns one entry per checkbox line in file order.
// Non-checkbox lines are skipped but still advance the 1-based line
// numbers used as IDs.
func parseTodoEntries(reader io.Reader) ([]todoEntry, error) {
	var entries []todoEntry
	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		checked, title, ok := todoItemChecked(strings.TrimSpace(line))
		if !ok {
			continue
		}
		entries = append(entries, todoEntry{
			ID:       lineNumber,
			Indent:   todoIndentWidth(line),
			Title:    title,
			Complete: checked,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read TODO.md: %w", err)
	}
	return entries, nil
}

// todoIndentWidth measures leading whitespace with tabs expanded so
// mixed indentation still nests deterministically.
func todoIndentWidth(line string) int {
	width := 0
	for _, char := range line {
		switch char {
		case ' ':
			width++
		case '\t':
			width += 8 - (width % 8)
		default:
			return width
		}
	}
	return width
}

// todoItemChecked reports whether a trimmed line is a Markdown checkbox,
// whether that checkbox is checked, and the checkbox title.
func todoItemChecked(line string) (checked bool, title string, ok bool) {
	item, found := strings.CutPrefix(line, "- ")
	if !found {
		return false, "", false
	}
	var marker string
	switch {
	case strings.HasPrefix(item, "[ ]"):
		marker = "[ ]"
	case strings.HasPrefix(item, "[x]"), strings.HasPrefix(item, "[X]"):
		checked = true
		marker = item[:3]
	default:
		return false, "", false
	}
	return checked, strings.TrimSpace(strings.TrimPrefix(item[len(marker):], " ")), true
}
