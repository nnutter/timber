package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutoContextReadsOnlyCapturedObjects(t *testing.T) {
	r := testRepo(t)
	file := filepath.Join(r.Dir, "code.go")
	require.NoError(t, os.WriteFile(file, []byte("package example\nconst Old = 1\n"), 0o600))
	gitTest(t, r.Dir, "add", "code.go")
	gitTest(t, r.Dir, "commit", "-m", "Original")
	base := gitTest(t, r.Dir, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(file, []byte("package example\nconst New = 1\n"), 0o600))
	gitTest(t, r.Dir, "commit", "-am", "Refactor: Rename constant")
	tip := gitTest(t, r.Dir, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(file, []byte("uncommitted content\n"), 0o600))
	reader := commitContext{repository: r, base: base, tip: tip}
	before, err := reader.ReadFile(t.Context(), "base", "code.go")
	require.NoError(t, err)
	require.Equal(t, "package example\nconst Old = 1\n", before)
	after, err := reader.ReadFile(t.Context(), "tip", "code.go")
	require.NoError(t, err)
	require.Equal(t, "package example\nconst New = 1\n", after)
	matches, err := reader.Search(t.Context(), "base", "Old")
	require.NoError(t, err)
	require.Equal(t, "code.go:2:const Old = 1\n", matches)
	matches, err = reader.Search(t.Context(), "tip", "Old")
	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestAutoContextRestrictsPathsSearchesAndOutput(t *testing.T) {
	r := testRepo(t)
	for _, name := range []string{"normal.go", ".env", "nested/credentials/data", "nested/.env.local"} {
		file := filepath.Join(r.Dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
		require.NoError(t, os.WriteFile(file, []byte("fixture-marker\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "large"), []byte(strings.Repeat("x", autoContextLimit+1)), 0o600))
	gitTest(t, r.Dir, "add", ".")
	gitTest(t, r.Dir, "commit", "-m", "Fixture")
	sha := gitTest(t, r.Dir, "rev-parse", "HEAD")
	reader := commitContext{repository: r, base: sha, tip: sha}
	for _, name := range []string{"/normal.go", "../normal.go", "nested/../normal.go", ".env", "nested/.env.local", "nested/credentials/data"} {
		_, err := reader.ReadFile(t.Context(), "tip", name)
		require.Error(t, err, "%s", name)
	}
	_, err := reader.ReadFile(t.Context(), "HEAD", "normal.go")
	require.ErrorContains(t, err, "base or tip")
	_, err = reader.ReadFile(t.Context(), "tip", "large")
	require.Error(t, err)
	for _, query := range []string{"", "multi\nline", strings.Repeat("x", 257)} {
		_, err := reader.Search(t.Context(), "tip", query)
		require.Error(t, err)
	}
	matches, err := reader.Search(t.Context(), "tip", "fixture-marker")
	require.NoError(t, err)
	require.Contains(t, matches, "normal.go:1:fixture-marker")
	require.NotContains(t, matches, ".env")
	require.NotContains(t, matches, "credentials")
}

func TestAutoContextReportsGitFailures(t *testing.T) {
	r := testRepo(t)
	_, err := r.autoGit(t.Context(), autoContextLimit, "cat-file", "blob", "missing:missing")
	require.ErrorContains(t, err, "read automatic review context")
	require.ErrorContains(t, err, "fatal:")
}
