package timber

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootInstallSkill(t *testing.T) {
	t.Parallel()
	runtime := testRuntimeForHome(resolvedTempDir(t), resolvedTempDir(t))
	// Use the shipped content, not a test-only placeholder.
	content, err := os.ReadFile(filepath.Join("..", "..", "skills", "timber", "SKILL.md"))
	require.NoError(t, err)
	runtime.SkillContent = string(content)
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "--install-skill")
	require.NoError(t, result.err, result.stderr)
	destination := filepath.Join(runtime.HomeDirectory, ".agents", "skills", "timber", "SKILL.md")
	installed, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, content, installed)
	assert.Contains(t, result.stderr, "installed timber skill")

	runtime.SkillContent = "replacement\n"
	result = runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "--install-skill")
	require.ErrorContains(t, result.err, "already exists")
	installed, err = os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, content, installed)

	result = runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "--install-skill", "--force")
	require.NoError(t, result.err, result.stderr)
	installed, err = os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, runtime.SkillContent, string(installed))
}

func TestRootInstallSkillUnavailable(t *testing.T) {
	t.Parallel()
	runtime := testRuntimeForHome(resolvedTempDir(t), resolvedTempDir(t))
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "--install-skill")
	require.ErrorContains(t, result.err, "skill content is not available")
	_, err := os.Stat(filepath.Join(runtime.HomeDirectory, ".agents"))
	require.True(t, os.IsNotExist(err))
}

func TestRootForceRequiresInstallSkill(t *testing.T) {
	t.Parallel()
	runtime := testRuntimeForHome(resolvedTempDir(t), resolvedTempDir(t))
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "--force")
	require.ErrorContains(t, result.err, "--force requires --install-skill")
}
