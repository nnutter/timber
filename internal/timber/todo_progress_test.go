package timber

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTodoProgressCountsChecklistItems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lines    []string
		expected todoProgress
	}{
		{
			name: "mixed checkboxes",
			lines: []string{
				"- [x] done",
				"- [ ] pending",
			},
			expected: todoProgress{Done: 1, Total: 2},
		},
		{
			name: "uppercase checkbox counts as done",
			lines: []string{
				"- [X] done",
			},
			expected: todoProgress{Done: 1, Total: 1},
		},
		{
			name: "nested and indented items count",
			lines: []string{
				"## Section",
				"  - [ ] nested",
				"    - [x] deeper",
			},
			expected: todoProgress{Done: 1, Total: 2},
		},
		{
			name: "non-checkbox lines are ignored",
			lines: []string{
				"# TODO",
				"- plain bullet",
				"- [-] not a checkbox",
				"text [x] inline",
			},
			expected: todoProgress{},
		},
		{
			name:     "empty checklist",
			lines:    nil,
			expected: todoProgress{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			progress, err := parseTodoProgress(strings.NewReader(strings.Join(testCase.lines, "\n")))
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, progress)
		})
	}
}

func TestTodoProgressStringRendersDoneOverTotal(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1/2", todoProgress{Done: 1, Total: 2}.String())
	assert.Equal(t, "0/0", todoProgress{}.String())
}

func TestReadTodoProgressReadsWorktreeGitDirectory(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()
	writeTodoFile(t, todoFilePath(gitDir), "- [x] done\n- [ ] pending\n")

	progress, err := readTodoProgress(gitDir)
	require.NoError(t, err)
	assert.Equal(t, todoProgress{Done: 1, Total: 2}, progress)
	assert.Equal(t, "1/2", progress.String())
}

func TestReadTodoProgressTreatsMissingFileAsEmpty(t *testing.T) {
	t.Parallel()
	progress, err := readTodoProgress(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, todoProgress{}, progress)
}

func TestTodoFilePathLivesInGitDirectory(t *testing.T) {
	t.Parallel()
	assert.Equal(t, filepath.Join("/repo/.git/worktrees/one", todoFileName), todoFilePath("/repo/.git/worktrees/one"))
}
