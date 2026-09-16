package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoListSortOrder(t *testing.T) {
	t.Parallel()
	home := resolvedTempDir(t)
	runtime := testRuntimeForHome(home, home)
	for _, repo := range []struct {
		name   string
		alias  string
		origin string
	}{
		{name: "alpha", origin: "git@github.com:zulu/project.git"},
		{name: "zeta", origin: "https://github.com/bravo/project.git"},
		{name: "beta", alias: "aardvark", origin: "https://github.com/zulu/other.git"},
		{name: "gamma", alias: "aardvark"},
		{name: "local"},
	} {
		barePath := runtime.bareRepoPath(repo.name)
		runGitCommand(t, home, "init", "--bare", barePath)
		if repo.origin != "" {
			runGitCommand(t, barePath, "remote", "add", "origin", repo.origin)
		}
		if repo.alias != "" {
			runGitCommand(t, barePath, "config", "--local", "timber.alias", repo.alias)
		}
	}

	for _, testCase := range []struct {
		name  string
		flags []string
		want  []string
	}{
		{name: "alias", want: []string{"local", "beta", "gamma", "zeta", "alpha"}},
		{name: "name", flags: []string{"--sort-name"}, want: []string{"alpha", "beta", "gamma", "local", "zeta"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			for _, quiet := range []string{"", "-q", "--quiet"} {
				t.Run("quiet="+quiet, func(t *testing.T) {
					args := append([]string{"repo", "list"}, testCase.flags...)
					if quiet != "" {
						args = append(args, quiet)
					}
					result := runTimberCommandWithRuntime(t, runtime, args...)
					require.NoError(t, result.err, result.stderr)
					if quiet != "" {
						assert.Equal(t, strings.Join(testCase.want, "\n")+"\n", result.stdout)
						return
					}
					previous := -1
					for _, name := range testCase.want {
						position := strings.Index(result.stdout, name)
						require.Greater(t, position, previous, "repository %s out of order in %s", name, result.stdout)
						previous = position
					}
				})
			}
		})
	}
}
