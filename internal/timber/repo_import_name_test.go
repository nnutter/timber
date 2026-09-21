package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRepoNameFromPathStripsGitSuffix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/main/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam"))
}
