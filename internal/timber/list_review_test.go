package timber

import (
	"crypto/sha256"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/gitreview/review"
)

func TestListReviewProgress(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	for _, name := range []string{"feature/reviewed", "feature/empty"} {
		require.NoError(t, repository.runTimber(t, "create", at(testRepoName, name)).err)
	}
	repository.commitFileInWorktree(t, "feature/reviewed", "first.txt", "first\n")
	runGitCommand(t, repository.worktreePath("feature/reviewed"), "commit", "--allow-empty", "--fixup=HEAD", "-m", "fixup")
	repository.commitFileInWorktree(t, "feature/reviewed", "second.txt", "second\n")
	service := review.Service{Repository: review.Repository{Dir: repository.worktreePath("feature/reviewed"), Environment: repository.runtime.Environment}}
	_, err := service.Approve(t.Context(), "HEAD~2", "HEAD~1")
	require.NoError(t, err)
	statePath := reviewStatePath(t, repository, "feature/reviewed")
	before, err := os.ReadFile(statePath)
	require.NoError(t, err)

	result := repository.runTimber(t, "ls")
	require.NoError(t, result.err, result.stderr)
	assert.Less(t, strings.Index(result.stdout, "Todo"), strings.Index(result.stdout, "Review"))
	assert.Less(t, strings.Index(result.stdout, "Review"), strings.Index(result.stdout, "Commit"))
	assert.Contains(t, result.stdout, "2/3")
	assert.Contains(t, result.stdout, "0/0")
	after, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	encoded := repository.runTimber(t, "ls", "--json")
	require.NoError(t, encoded.err, encoded.stderr)
	var records []struct {
		Name   string `json:"name"`
		Review *struct {
			Approved int `json:"approved"`
			Total    int `json:"total"`
		} `json:"review"`
	}
	require.NoError(t, json.Unmarshal([]byte(encoded.stdout), &records))
	for _, record := range records {
		require.NotNil(t, record.Review)
		switch record.Name {
		case "feature/reviewed":
			assert.Equal(t, 2, record.Review.Approved)
			assert.Equal(t, 3, record.Review.Total)
		case "feature/empty":
			assert.Zero(t, record.Review.Total)
		}
	}
}

func TestListReviewProgressUnknown(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, "feature/healthy")).err)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, "feature/no-upstream")).err)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, "feature/corrupt")).err)
	repository.commitFileInWorktree(t, "feature/corrupt", "file.txt", "change\n")
	runGitCommand(t, repository.barePath, "branch", "--unset-upstream", "feature/no-upstream")
	statePath := reviewStatePath(t, repository, "feature/corrupt")
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o700))
	require.NoError(t, os.WriteFile(statePath, []byte("{"), 0o600))
	result := repository.runTimber(t, "ls", "--json")
	require.NoError(t, result.err, result.stderr)
	var records []struct {
		Name   string `json:"name"`
		Review *struct {
			Total int `json:"total"`
		} `json:"review"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.stdout), &records))
	for _, record := range records {
		switch record.Name {
		case "feature/healthy":
			require.NotNil(t, record.Review)
			assert.Zero(t, record.Review.Total)
		case "feature/no-upstream", "feature/corrupt":
			assert.Nil(t, record.Review)
		}
	}
}

func TestListReviewProgressIgnoresAmbientGitEnvironment(t *testing.T) {
	decoy := resolvedTempDir(t)
	runGitCommand(t, decoy, "init", "-b", "main")
	runGitCommand(t, decoy, "commit", "--allow-empty", "-m", "decoy")
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_DIFF_OPTS", "--ignore-space-change")
	repository := newTestRepository(t)
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, "feature/isolated")).err)
	repository.commitFileInWorktree(t, "feature/isolated", "file.txt", "one\n")
	service := review.Service{Repository: review.Repository{Dir: repository.worktreePath("feature/isolated"), Environment: repository.runtime.Environment}}
	_, err := service.ApproveAll(t.Context())
	require.NoError(t, err)

	result := repository.runTimber(t, "ls", "--json")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, `"approved":1,"total":1`)
	assert.Equal(t, "main", strings.TrimSpace(runGitCommand(t, decoy, "branch", "--show-current")))
}

func reviewStatePath(t *testing.T, repository testRepository, branch string) string {
	t.Helper()
	common := strings.TrimSpace(runGitCommand(t, repository.worktreePath(branch), "rev-parse", "--path-format=absolute", "--git-common-dir"))
	return filepath.Join(common, "timber-review", "state", fmt.Sprintf("%x.json", sha256.Sum256([]byte(branch))))
}
