package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePullRequestListFormatsCheckMarks(t *testing.T) {
	t.Parallel()

	output := []byte(`[
		{"number":42,"headRefName":"feature/green","state":"OPEN","statusCheckRollup":[{"conclusion":"SUCCESS","status":"COMPLETED"}]},
		{"number":43,"headRefName":"feature/red","state":"OPEN","statusCheckRollup":[{"conclusion":"FAILURE","status":"COMPLETED"}]},
		{"number":44,"headRefName":"feature/running","state":"OPEN","statusCheckRollup":[{"conclusion":"","status":"IN_PROGRESS"}]},
		{"number":45,"headRefName":"feature/nochecks","state":"OPEN","statusCheckRollup":[]}
	]`)

	pullRequests, err := parsePullRequestList(output)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"feature/green":    "#42 ✓",
		"feature/red":      "#43 ✗",
		"feature/running":  "#44 …",
		"feature/nochecks": "#45",
	}, pullRequests)
}

func TestParsePullRequestListSkipsInvalidEntries(t *testing.T) {
	t.Parallel()

	output := []byte(`[
		{"number":0,"headRefName":"feature/zero","state":"OPEN","statusCheckRollup":[]},
		{"number":46,"headRefName":"","state":"OPEN","statusCheckRollup":[]},
		{"number":47,"headRefName":"feature/dup","state":"OPEN","statusCheckRollup":[]},
		{"number":48,"headRefName":"feature/dup","state":"OPEN","statusCheckRollup":[]}
	]`)

	pullRequests, err := parsePullRequestList(output)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"feature/dup": "#47"}, pullRequests)
}

func TestParsePullRequestListRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	_, err := parsePullRequestList([]byte(`{`))
	require.ErrorContains(t, err, "decode gh pull request list")
}
