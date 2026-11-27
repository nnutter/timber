package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusShowsCurrentWorktreeWithoutIdentityColumns(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	const branch = "feature/current-status"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branch)).err)
	path := repository.worktreePath(branch)
	repository.writeFileInWorktree(t, branch, "untracked", "dirty\n")
	writeTodoFile(t, todoPathForWorktree(t, path), "- [x] Done\n- [ ] Open\n")
	writeTodoFile(t, filepath.Join(repository.barePath, "TODO.md"), "- [x] Done\n- [x] Also done\n- [ ] Open\n")
	repository.runtime.GhExecutable = installFakeGh(t)
	repository.runtime = withTestEnvironment(repository.runtime,
		`FAKE_GH_PR_JSON=[{"number":42,"headRefName":"feature/current-status","state":"OPEN"}]`,
	)

	result := repository.runTimberFrom(t, path, "status")
	require.NoError(t, result.err, result.stderr)
	output := ansi.Strip(result.stdout)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 5)
	header := strings.Split(strings.Trim(lines[1], "│"), "│")
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	assert.Equal(t, []string{"Status", "TODO", "Repo TODO", "Review", "Commit", "Dirty", "PR"}, header)
	row := strings.Split(strings.Trim(lines[3], "│"), "│")
	require.Len(t, row, 7)
	assert.Equal(t, "1/2", strings.TrimSpace(row[1]))
	assert.Equal(t, "2/3", strings.TrimSpace(row[2]))
	assert.Contains(t, output, "dirty")
	assert.Contains(t, output, "#42")
	assert.NotContains(t, output, branch)
}
