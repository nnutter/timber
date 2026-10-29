package list

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportJSONPreservesWorktreeFields(t *testing.T) {
	t.Parallel()
	worktrees := []JSONWorktree{
		{
			Name: "feature", Repo: "repo", Path: "/work/feature",
			Upstream: "origin/main", Ahead: 2, Behind: 3, Commit: "abcdef012345",
			Clean: true, Merged: true, StatusError: true, TodoDone: 1, TodoTotal: 4,
			PullRequest: "#42 ✓", Review: &JSONReviewProgress{Approved: 2, Total: 5},
		},
		{Name: "unreviewed"},
	}
	var output bytes.Buffer
	require.NoError(t, ReportJSON(&output, worktrees))
	assert.Equal(t, "[{\"name\":\"feature\",\"repo\":\"repo\",\"path\":\"/work/feature\",\"upstream\":\"origin/main\",\"ahead\":2,\"behind\":3,\"commit\":\"abcdef012345\",\"clean\":true,\"merged\":true,\"statusError\":true,\"todoDone\":1,\"todoTotal\":4,\"pullRequest\":\"#42 ✓\",\"review\":{\"approved\":2,\"total\":5}},{\"name\":\"unreviewed\",\"repo\":\"\",\"path\":\"\",\"upstream\":\"\",\"ahead\":0,\"behind\":0,\"commit\":\"\",\"clean\":false,\"merged\":false,\"statusError\":false,\"todoDone\":0,\"todoTotal\":0,\"pullRequest\":\"\"}]\n", output.String())
}

func TestReportJSONEmptyWorktrees(t *testing.T) {
	t.Parallel()
	for _, worktrees := range [][]JSONWorktree{nil, {}} {
		var output bytes.Buffer
		require.NoError(t, ReportJSON(&output, worktrees))
		assert.Equal(t, "[]\n", output.String())
	}
}

type failingJSONWriter struct{ err error }

func (x failingJSONWriter) Write([]byte) (int, error) { return 0, x.err }

func TestReportJSONReturnsWriterError(t *testing.T) {
	t.Parallel()
	writeErr := errors.New("write failed")
	assert.ErrorIs(t, ReportJSON(failingJSONWriter{err: writeErr}, nil), writeErr)
}
