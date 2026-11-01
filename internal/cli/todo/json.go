package todo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/nnutter/timber/internal/timber/todo"
)

// todoJSONSummary counts TODO.md items by status.
type todoJSONSummary struct {
	Incomplete int `json:"incomplete"`
	Complete   int `json:"complete"`
}

// todoJSONItem is one TODO.md checkbox. ID is the 1-based line number in
// TODO.md, and DependsOn is the ID of the nearest preceding checkbox with
// less indentation, or null for a top-level item.
type todoJSONItem struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	DependsOn *int   `json:"depends_on"`
	Complete  bool   `json:"complete"`
}

// todoJSONOutput is the `todo --json` document.
type todoJSONOutput struct {
	Summary todoJSONSummary `json:"summary"`
	Todos   []todoJSONItem  `json:"todos"`
}

// readTodoJSON parses a TODO list into its JSON document. A missing file is
// an empty checklist because `timber todo` creates the file lazily.
func readTodoJSON(path string) (todoJSONOutput, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return todoJSONOutput{Todos: []todoJSONItem{}}, nil
	}
	if err != nil {
		return todoJSONOutput{}, fmt.Errorf("open %s: %w", path, err)
	}
	return todoJSONFromContents(string(contents)), nil
}

// todoJSONFromContents converts raw TODO.md text to its JSON document.
func todoJSONFromContents(contents string) todoJSONOutput {
	entries, err := todo.ParseEntries(strings.NewReader(contents))
	if err != nil {
		// parseTodoEntries only fails on scanner I/O errors, which cannot
		// happen for an in-memory string; fall through with no items.
		return todoJSONOutput{Todos: []todoJSONItem{}}
	}
	return todoEntriesToJSON(entries)
}

// todoEntriesToJSON resolves indentation into depends_on links and counts
// the summary. Each item's parent is the nearest preceding item with less
// indentation, tracked with a stack.
func todoEntriesToJSON(entries []todo.Entry) todoJSONOutput {
	output := todoJSONOutput{Todos: []todoJSONItem{}}
	var stack []todo.Entry
	for _, entry := range entries {
		for len(stack) > 0 && !isLess(stack[len(stack)-1].Indent, entry.Indent) {
			stack = stack[:len(stack)-1]
		}
		item := todoJSONItem{
			ID:       entry.ID,
			Title:    entry.Title,
			Complete: entry.Complete,
		}
		if len(stack) > 0 {
			parentID := stack[len(stack)-1].ID
			item.DependsOn = &parentID
		}
		output.Todos = append(output.Todos, item)
		stack = append(stack, entry)
		if entry.Complete {
			output.Summary.Complete++
		} else {
			output.Summary.Incomplete++
		}
	}
	return output
}
