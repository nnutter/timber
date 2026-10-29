package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReviewInstallSkill(t *testing.T) {
	t.Parallel()
	runtime := testRuntimeForHome(resolvedTempDir(t), resolvedTempDir(t))
	content, err := os.ReadFile(filepath.Join("..", "..", "skills", "timber-review", "SKILL.md"))
	require.NoError(t, err)
	runtime.ReviewSkillContent = string(content)
	destination := filepath.Join(runtime.HomeDirectory, ".agents", "skills", "timber-review", "SKILL.md")
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "review", "--install-skill")
	require.NoError(t, result.err, result.stderr)
	installed, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, content, installed)

	runtime.ReviewSkillContent = "replacement\n"
	result = runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "review", "--install-skill")
	require.ErrorContains(t, result.err, "already exists")
	installed, err = os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, content, installed)
	result = runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "review", "--install-skill", "--force")
	require.NoError(t, result.err, result.stderr)
	installed, err = os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, runtime.ReviewSkillContent, string(installed))
}
