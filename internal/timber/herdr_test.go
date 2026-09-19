package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHerdrSpaceRejectsDuplicateFields(t *testing.T) {
	t.Parallel()

	output := []byte(`{"result":{"workspace":{"workspace_id":"w1","workspace_id":"w1"},"tab":{"tab_id":"w1:t1"},"root_pane":{"pane_id":"w1:p1"}}}`)

	_, err := parseHerdrSpace(Runtime{}, output, "/tmp/worktree", "feature")
	require.Error(t, err)
}

func TestParseHerdrSpaceReadsAlreadyOpen(t *testing.T) {
	t.Parallel()

	output := []byte(`{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"},"already_open":true}}`)

	space, err := parseHerdrSpace(Runtime{}, output, "/tmp/worktree", "feature")
	require.NoError(t, err)
	assert.True(t, space.alreadyOpen)
	assert.Equal(t, "w2", space.workspaceID)
}

func TestParseHerdrSpaceDefaultsToNotAlreadyOpen(t *testing.T) {
	t.Parallel()

	output := []byte(`{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"}}}`)

	space, err := parseHerdrSpace(Runtime{}, output, "/tmp/worktree", "feature")
	require.NoError(t, err)
	assert.False(t, space.alreadyOpen)
}

func TestParseHerdrWorkspaceID(t *testing.T) {
	t.Parallel()

	id, err := parseHerdrWorkspaceID([]byte(`{"result":{"workspace":{"workspace_id":"w1"}}}`))
	require.NoError(t, err)
	assert.Equal(t, "w1", id)

	_, err = parseHerdrWorkspaceID([]byte(`{"result":{"workspace":{}}}`))
	require.ErrorContains(t, err, "no workspace ID")

	_, err = parseHerdrWorkspaceID([]byte(`{`))
	require.ErrorContains(t, err, "decode herdr workspace create response")
}

func TestParseHerdrWorktreeListParent(t *testing.T) {
	t.Parallel()

	id, err := parseHerdrWorktreeListParent([]byte(`{"result":{"source":{"source_workspace_id":"w7"}}}`))
	require.NoError(t, err)
	assert.Equal(t, "w7", id)

	id, err = parseHerdrWorktreeListParent([]byte(`{"result":{"source":{"source_workspace_id":null}}}`))
	require.NoError(t, err)
	assert.Empty(t, id)

	_, err = parseHerdrWorktreeListParent([]byte(`{`))
	require.ErrorContains(t, err, "decode herdr worktree list response")
}

func TestParseHerdrWorkspaceLabel(t *testing.T) {
	t.Parallel()

	label, err := parseHerdrWorkspaceLabel([]byte(`{"result":{"workspace":{"workspace_id":"w7","label":"repo.git"}}}`), "w7")
	require.NoError(t, err)
	assert.Equal(t, "repo.git", label)

	_, err = parseHerdrWorkspaceLabel([]byte(`{"result":{"workspace":{"workspace_id":"w8","label":"repo"}}}`), "w7")
	require.ErrorContains(t, err, "unexpected workspace ID")

	_, err = parseHerdrWorkspaceLabel([]byte(`{"result":{"workspace":{}}}`), "w7")
	require.ErrorContains(t, err, "no workspace ID")

	_, err = parseHerdrWorkspaceLabel([]byte(`{`), "w7")
	require.ErrorContains(t, err, "decode herdr workspace get response")
}
