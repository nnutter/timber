package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUsesRepositoryNameBeforeFallingBackToRandomName(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	first := testRepository.runTimber(t, "create", at(testRepoName, ""))
	require.NoError(t, first.err, first.stderr)
	second := testRepository.runTimber(t, "create", at(testRepoName, ""))
	require.NoError(t, second.err, second.stderr)

	assert.Equal(t, testRepository.worktreePath(testRepoName), strings.TrimSpace(first.stdout))
	randomPath := strings.TrimSpace(second.stdout)
	name := filepath.Base(filepath.Dir(randomPath))
	assert.Regexp(t, `^[a-z]+-[a-z]+$`, name)
	testRepository.assertPathPresent(t, randomPath)
}
