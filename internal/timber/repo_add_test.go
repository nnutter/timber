package timber

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAddAlias(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"", "custom-alias"} {
		t.Run("alias="+alias, func(t *testing.T) {
			home := resolvedTempDir(t)
			runtime := testRuntimeForHome(home, home)
			remote := filepath.Join(resolvedTempDir(t), "remote.git")
			runGitCommand(t, home, "init", "--bare", remote)
			seedBareRemote(t, remote)
			args := []string{"repo", "add", "--name", "demo"}
			if alias != "" {
				args = append(args, "--alias", alias)
			}
			result := runTimberCommandWithRuntime(t, runtime, append(args, remote)...)
			require.NoError(t, result.err, result.stderr)
			repo := registeredRepo{Name: "demo", BarePath: runtime.bareRepoPath("demo")}
			if alias != "" {
				assert.Equal(t, alias+"\n", runGitCommand(t, repo.BarePath, "config", "--local", "--get", "timber.alias"))
			} else {
				assert.Equal(t, remote, repo.alias(runtime, remote))
			}
			runGitCommand(t, repo.BarePath, "remote", "set-url", "origin", "git@github.com:nnutter/agentctl.git")
			want := alias
			if want == "" {
				want = "nnutter/agentctl"
			}
			assert.Equal(t, want, repo.alias(runtime, repo.originURL(runtime)))
			list := runTimberCommandWithRuntime(t, runtime, "repo", "list")
			require.NoError(t, list.err)
			assert.Contains(t, list.stdout, "Alias")
			assert.Contains(t, list.stdout, want)
			quiet := runTimberCommandWithRuntime(t, runtime, "repo", "list", "-q")
			require.NoError(t, quiet.err)
			assert.Equal(t, "demo\n", quiet.stdout)
			runGitCommand(t, repo.BarePath, "remote", "remove", "origin")
			assert.Equal(t, alias, repo.alias(runtime, repo.originURL(runtime)))
		})
	}
}
