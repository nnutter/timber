package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAddRemoteURL(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		input string
		want  string
	}{
		{"nnutter/timber", "git@github.com:nnutter/timber"},
		{"nnutter/timber.git", "git@github.com:nnutter/timber"},
		{"https://github.com/nnutter/timber", "https://github.com/nnutter/timber"},
		{"https://github.com/nnutter/timber.git", "https://github.com/nnutter/timber.git"},
		{"git@github.com:nnutter/timber.git", "git@github.com:nnutter/timber.git"},
	} {
		t.Run(fixture.input, func(t *testing.T) {
			t.Parallel()

			runtime := testRuntime(t)
			home := runtime.HomeDirectory
			remote := filepath.Join(resolvedTempDir(t), "remote.git")
			runGitCommand(t, home, "init", "--bare", remote)
			seedImportRemote(t, remote)
			// Redirect only the expected URL to a local remote. A different URL
			// must fail instead of attempting a network connection.
			runtime.Environment = append(runtime.Environment,
				"GIT_CONFIG_COUNT=1",
				"GIT_CONFIG_KEY_0=url."+remote+".insteadOf",
				"GIT_CONFIG_VALUE_0="+fixture.want,
				"GIT_ALLOW_PROTOCOL=file",
			)

			result := runTimberCommandWithRuntime(t, runtime, "repo", "add", fixture.input)
			require.NoError(t, result.err, result.stderr)
			assert.Equal(t, fixture.want+"\n", runGitCommand(t, filepath.Join(home, "worktrees", "nnutter", "timber", ".git"),
				"config", "--local", "--get", "remote.origin.url"))
		})
	}
}

func TestRepoAddAlias(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"", "custom-alias"} {
		t.Run("alias="+alias, func(t *testing.T) {
			runtime := testRuntime(t)
			home := runtime.HomeDirectory
			remote := filepath.Join(resolvedTempDir(t), "remote.git")
			runGitCommand(t, home, "init", "--bare", remote)
			seedImportRemote(t, remote)
			args := []string{"repo", "add", "--name", "demo"}
			if alias != "" {
				args = append(args, "--alias", alias)
			}
			result := runTimberCommandWithRuntime(t, runtime, append(args, remote)...)
			require.NoError(t, result.err, result.stderr)
			barePath := filepath.Join(home, "worktrees", "demo", ".git")
			if alias != "" {
				assert.Equal(t, alias+"\n", runGitCommand(t, barePath, "config", "--local", "--get", "timber.alias"))
			}
			assertList := func(wantAlias, wantOrigin string) {
				t.Helper()
				list := runTimberCommandWithRuntime(t, runtime, "repo", "list")
				require.NoError(t, list.err, list.stderr)
				lines := strings.Split(strings.TrimSpace(list.stdout), "\n")
				require.Len(t, lines, 5)
				cells := strings.Split(lines[3], "│")
				require.Len(t, cells, 5)
				assert.Equal(t, "demo", strings.TrimSpace(cells[1]))
				assert.Equal(t, wantAlias, strings.TrimSpace(cells[2]))
				assert.Equal(t, wantOrigin, strings.TrimSpace(cells[3]))
			}
			initialAlias := alias
			if initialAlias == "" {
				initialAlias = remote
			}
			assertList(initialAlias, remote)
			origin := "git@github.com:nnutter/agentctl.git"
			runGitCommand(t, barePath, "remote", "set-url", "origin", origin)
			want := alias
			if want == "" {
				want = "nnutter/agentctl"
			}
			assertList(want, origin)
			quiet := runTimberCommandWithRuntime(t, runtime, "repo", "list", "-q")
			require.NoError(t, quiet.err)
			assert.Equal(t, "demo\n", quiet.stdout)
			runGitCommand(t, barePath, "remote", "remove", "origin")
			assertList(alias, "")
		})
	}
}
