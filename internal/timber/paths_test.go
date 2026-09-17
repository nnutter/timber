package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRepoNameStripsGitSuffix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "roam", normalizeRepoName("roam.git"))
	assert.Equal(t, "roam", normalizeRepoName(" roam.git "))
	assert.Equal(t, "roam", normalizeRepoName("roam"))
}
