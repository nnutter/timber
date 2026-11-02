package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A message fingerprint must survive rewrites that keep the message and must
// change when a review of the stack would show a different message.
func TestStackMessageFingerprint(t *testing.T) {
	feature := func(t *testing.T) Repository {
		t.Helper()
		r := testRepo(t)
		require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "file"), []byte("one\n"), 0o600))
		gitTest(t, r.Dir, "add", ".")
		gitTest(t, r.Dir, "commit", "-m", "Feature")
		return r
	}
	t.Run("rebase keeps the message", func(t *testing.T) {
		r := feature(t)
		before := stackMessageFingerprint(t, r)
		gitTest(t, r.Dir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "other"), []byte("upstream\n"), 0o600))
		gitTest(t, r.Dir, "add", ".")
		gitTest(t, r.Dir, "commit", "-m", "Unrelated upstream change")
		gitTest(t, r.Dir, "checkout", "feature")
		gitTest(t, r.Dir, "rebase", "main")
		require.Equal(t, before, stackMessageFingerprint(t, r))
	})
	t.Run("patch change keeps the message", func(t *testing.T) {
		r := feature(t)
		before := stackMessageFingerprint(t, r)
		require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "file"), []byte("two\n"), 0o600))
		gitTest(t, r.Dir, "add", ".")
		gitTest(t, r.Dir, "commit", "-m", "fixup! Feature")
		require.Equal(t, before, stackMessageFingerprint(t, r))
	})
	t.Run("amend with the target message keeps it", func(t *testing.T) {
		r := feature(t)
		before := stackMessageFingerprint(t, r)
		t.Setenv("GIT_EDITOR", "true")
		gitTest(t, r.Dir, "commit", "--allow-empty", "--fixup=amend:HEAD")
		require.Equal(t, before, stackMessageFingerprint(t, r))
	})
	t.Run("body change is a new message", func(t *testing.T) {
		r := feature(t)
		before := stackMessageFingerprint(t, r)
		gitTest(t, r.Dir, "commit", "--amend", "-m", "Feature", "-m", "Explains the change")
		require.NotEqual(t, before, stackMessageFingerprint(t, r))
	})
	t.Run("applied amend keeps the appended message", func(t *testing.T) {
		r := feature(t)
		before := stackMessageFingerprint(t, r)
		gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "amend! Feature", "-m", "Improved message")
		appended := stackMessageFingerprint(t, r)
		require.NotEqual(t, before, appended)
		t.Setenv("GIT_SEQUENCE_EDITOR", ":")
		gitTest(t, r.Dir, "rebase", "-i", "--autosquash", "main")
		require.Equal(t, "Improved message", gitTest(t, r.Dir, "log", "-1", "--format=%s"))
		require.Equal(t, appended, stackMessageFingerprint(t, r))
	})
}

func stackMessageFingerprint(t *testing.T, r Repository) string {
	t.Helper()
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, view.Stacks)
	for _, stack := range view.Stacks {
		require.NotEmpty(t, stack.MessageHash, "stack %s", stack.Subject)
	}
	return view.Stacks[0].MessageHash
}
