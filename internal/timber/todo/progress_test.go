package todo

import (
	"os"
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
		expected Progress
	}{
		{
			name: "mixed checkboxes",
			lines: []string{
				"- [x] done",
				"- [ ] pending",
			},
			expected: Progress{Done: 1, Total: 2},
		},
		{
			name: "uppercase checkbox counts as done",
			lines: []string{
				"- [X] done",
			},
			expected: Progress{Done: 1, Total: 1},
		},
		{
			name: "nested and indented items count",
			lines: []string{
				"## Section",
				"  - [ ] nested",
				"    - [x] deeper",
			},
			expected: Progress{Done: 1, Total: 2},
		},
		{
			name: "non-checkbox lines are ignored",
			lines: []string{
				"# TODO",
				"- plain bullet",
				"- [-] not a checkbox",
				"text [x] inline",
			},
			expected: Progress{},
		},
		{
			name:     "empty checklist",
			lines:    nil,
			expected: Progress{},
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
	assert.Equal(t, "1/2", Progress{Done: 1, Total: 2}.String())
	assert.Equal(t, "0/0", Progress{}.String())
}

func TestReadTodoProgressReadsWorktreeGitDirectory(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()
	require.NoError(t, os.WriteFile(Path(gitDir), []byte("- [x] done\n- [ ] pending\n"), 0o600))

	progress, err := ReadProgress(gitDir)
	require.NoError(t, err)
	assert.Equal(t, Progress{Done: 1, Total: 2}, progress)
	assert.Equal(t, "1/2", progress.String())
}

func TestReadTodoProgressTreatsMissingFileAsEmpty(t *testing.T) {
	t.Parallel()
	progress, err := ReadProgress(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, Progress{}, progress)
}

func TestTodoFilePathLivesInGitDirectory(t *testing.T) {
	t.Parallel()
	assert.Equal(t, filepath.Join("/repo/.git/worktrees/one", "TODO.md"), Path("/repo/.git/worktrees/one"))
}
