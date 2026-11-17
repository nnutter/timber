package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRepoNameFromPathStripsGitSuffix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/main/roam.git"))
	assert.Equal(t, "roam", defaultRepoNameFromPath("/tmp/src/roam"))
}

func TestBuildImportPlanStripsConfiguredOwner(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	runtime := repository.runtime
	executable, err := os.Executable()
	require.NoError(t, err)
	runtime.TrashExecutable = executable

	sourcePath := filepath.Join(resolvedTempDir(t), "project")
	runGitCommand(t, filepath.Dir(sourcePath), "clone", repository.remotePath, sourcePath)
	runGitCommand(t, sourcePath, "remote", "set-url", "origin", "git@github.com:nnutter/project.git")

	plan, err := runtime.buildImportPlan(sourcePath, "", []string{"NNutter"})
	require.NoError(t, err)
	assert.Equal(t, "project", plan.repoName)

	plan, err = runtime.buildImportPlan(sourcePath, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "nnutter/project", plan.repoName)
}
