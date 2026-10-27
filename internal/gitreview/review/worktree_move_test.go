package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func movedReviewCheckout(t *testing.T) (Repository, string, string, string) {
	t.Helper()
	r := testRepo(t)
	oldPath, err := WorktreePath(r.Dir)
	require.NoError(t, err)
	gitTest(t, r.Dir, "worktree", "add", "--detach", oldPath, "HEAD")
	oldGitDir := gitTest(t, oldPath, "rev-parse", "--absolute-git-dir")
	newDir := filepath.Join(t.TempDir(), "moved")
	require.NoError(t, os.Rename(r.Dir, newDir))
	r.Dir = newDir
	path, err := WorktreePath(r.Dir)
	require.NoError(t, err)
	registration := filepath.Join(newDir, ".git", "worktrees", filepath.Base(oldGitDir))
	return r, path, oldPath, registration
}

func TestPrepareMovedReviewCheckout(t *testing.T) {
	r, path, _, _ := movedReviewCheckout(t)
	file := filepath.Join(path, "incidental")
	require.NoError(t, os.WriteFile(file, []byte("discard"), 0o600))
	tip := gitTest(t, r.Dir, "rev-parse", "HEAD")
	require.NoError(t, prepareWorktree(t.Context(), r, path, Stack{Tip: tip}))
	_, err := os.Stat(file)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, validateReviewCheckout(t.Context(), r, path))
}

func TestPrepareMovedReviewCheckoutRejectsUnsafeState(t *testing.T) {
	for _, scenario := range []string{"live old checkout", "wrong pointer", "wrong backlink", "attached checkout", "duplicate registration"} {
		t.Run(scenario, func(t *testing.T) {
			r, path, oldPath, registration := movedReviewCheckout(t)
			file := filepath.Join(path, "keep")
			require.NoError(t, os.WriteFile(file, []byte("keep"), 0o600))
			switch scenario {
			case "live old checkout":
				gitfile, err := os.ReadFile(filepath.Join(path, ".git"))
				require.NoError(t, err)
				oldGitDir, ok := strings.CutPrefix(strings.TrimSpace(string(gitfile)), "gitdir: ")
				require.True(t, ok)
				oldCommon := filepath.Dir(filepath.Dir(oldGitDir))
				oldRegistration := filepath.Join(oldCommon, "worktrees", filepath.Base(oldGitDir))
				require.NoError(t, os.MkdirAll(filepath.Dir(oldRegistration), 0o700))
				require.NoError(t, os.CopyFS(oldRegistration, os.DirFS(registration)))
				require.NoError(t, os.MkdirAll(oldPath, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(oldPath, ".git"), gitfile, 0o600))
			case "wrong pointer":
				require.NoError(t, os.WriteFile(filepath.Join(path, ".git"), []byte("gitdir: /wrong/place\n"), 0o600))
			case "wrong backlink":
				require.NoError(t, os.WriteFile(filepath.Join(registration, "gitdir"), []byte(filepath.Join(t.TempDir(), ".git")+"\n"), 0o600))
			case "attached checkout":
				require.NoError(t, os.WriteFile(filepath.Join(registration, "HEAD"), []byte("ref: refs/heads/feature\n"), 0o600))
			case "duplicate registration":
				require.NoError(t, os.CopyFS(registration+"-duplicate", os.DirFS(registration)))
			}
			gitfile := filepath.Join(path, ".git")
			beforeGitfile, err := os.ReadFile(gitfile)
			require.NoError(t, err)
			backlink := filepath.Join(registration, "gitdir")
			beforeBacklink, err := os.ReadFile(backlink)
			require.NoError(t, err)
			tip := gitTest(t, r.Dir, "rev-parse", "HEAD")
			require.Error(t, prepareWorktree(t.Context(), r, path, Stack{Tip: tip}))
			afterGitfile, err := os.ReadFile(gitfile)
			require.NoError(t, err)
			require.Equal(t, beforeGitfile, afterGitfile)
			afterBacklink, err := os.ReadFile(backlink)
			require.NoError(t, err)
			require.Equal(t, beforeBacklink, afterBacklink)
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, "keep", string(data))
		})
	}
}
