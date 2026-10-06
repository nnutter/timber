package timber

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCustomPathPreservesExplicitNameAcrossLifecycle(t *testing.T) {
	t.Parallel()
	for _, relative := range []bool{false, true} {
		label := "absolute"
		if relative {
			label = "relative"
		}
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepository(t)
			path := filepath.Join(repo.home, "custom projects", "checkout")
			argument := path
			if relative {
				argument = filepath.Join("custom projects", "checkout")
			}
			const name = "feature/custom"
			created := repo.runTimber(t, "create", at(testRepoName, name), argument)
			require.NoError(t, created.err, created.stderr)
			assert.Equal(t, path+"\n", created.stdout)
			assert.Equal(t, name, strings.TrimSpace(runGitCommand(t, path, "branch", "--show-current")))
			assert.NoDirExists(t, repo.worktreePath(name))

			// A subsequent command has no in-memory knowledge of create.
			listed := repo.runTimber(t, "list", "--json")
			require.NoError(t, listed.err, listed.stderr)
			var records []listJSONWorktree
			require.NoError(t, json.Unmarshal([]byte(listed.stdout), &records))
			require.Len(t, records, 1)
			assert.Equal(t, name, records[0].Name)
			assert.Equal(t, path, records[0].Path)

			duplicate := repo.runTimber(t, "create", at(testRepoName, name), filepath.Join(repo.home, "other"))
			require.Error(t, duplicate.err)
			assert.Contains(t, duplicate.err.Error(), "already exists")

			renamed := repo.runTimber(t, "repo", "rename", testRepoName, "renamed")
			require.NoError(t, renamed.err, renamed.stderr)
			switched := repo.runTimber(t, "switch", name)
			require.NoError(t, switched.err, switched.stderr)
			assert.Equal(t, path+"\n", switched.stdout)
			assert.Contains(t, runCompleteWithRuntime(t, repo.runtime, "switch", "feature/c"), name+"\n")

			removed := repo.runTimberFrom(t, path, "remove")
			require.NoError(t, removed.err, removed.stderr)
			assert.NoDirExists(t, path)
			assert.DirExists(t, filepath.Dir(path))
		})
	}
}

func TestCreateCustomPathWithGeneratedName(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	path := filepath.Join(repo.home, "custom", "checkout")
	created := repo.runTimber(t, "create", at(testRepoName, ""), path)
	require.NoError(t, created.err, created.stderr)
	branch := strings.TrimSpace(runGitCommand(t, path, "branch", "--show-current"))
	assert.NotEmpty(t, branch)
	assert.NotEqual(t, "checkout", branch)
	switched := repo.runTimber(t, "switch", branch)
	require.NoError(t, switched.err, switched.stderr)
	assert.Equal(t, path+"\n", switched.stdout)
}

func TestCreateCustomPathChecksExistingDestination(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"empty", "nonempty", "file", "missing"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepository(t)
			path := filepath.Join(repo.home, "checkout")
			switch kind {
			case "empty", "nonempty":
				require.NoError(t, os.Mkdir(path, 0o755))
				if kind == "nonempty" {
					require.NoError(t, os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0o600))
				}
			case "file":
				require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
			case "missing":
				path = ""
			}
			result := repo.runTimber(t, "create", at(testRepoName, "custom"), path)
			if kind == "empty" {
				require.NoError(t, result.err, result.stderr)
				assert.FileExists(t, filepath.Join(path, ".git"))
				return
			}
			require.Error(t, result.err)
			_, err := runGitCommandResult(repo.barePath, "show-ref", "--verify", "refs/heads/custom")
			assert.Error(t, err)
		})
	}
}
