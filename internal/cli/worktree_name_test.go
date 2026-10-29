package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGeneratesRandomNameWhenOmitted(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	result := testRepository.runTimber(t, "create", at(testRepoName, ""))
	require.NoError(t, result.err, result.stderr)

	worktreePath := strings.TrimSpace(result.stdout)
	name := filepath.Base(filepath.Dir(worktreePath))
	assert.Regexp(t, `^[a-z]+-[a-z]+$`, name)
	testRepository.assertPathPresent(t, testRepository.worktreePath(name))
}

func TestCreateGeneratesDistinctNamesWhenOmittedTwice(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)

	first := testRepository.runTimber(t, "create", at(testRepoName, ""))
	require.NoError(t, first.err, first.stderr)
	second := testRepository.runTimber(t, "create", at(testRepoName, ""))
	require.NoError(t, second.err, second.stderr)

	assert.NotEqual(t, strings.TrimSpace(first.stdout), strings.TrimSpace(second.stdout))
}
