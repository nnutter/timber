package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoImportRegistersBareAndMovesMainWorktree(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	fixture.assertImportedBareSetup(t, "project")

	// The main worktree becomes a managed worktree under timber.
	mainTarget := fixture.managedWorktreePath("project", "main")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")

	// The source checkout is gone.
	_, err := os.Stat(fixture.clonePath)
	require.ErrorIs(t, err, os.ErrNotExist)

	listResult := runTimberFromWithRuntime(t, fixture.runtime, fixture.runtime.HomeDirectory, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
}

func TestRepoImportSummaryShowsEachMove(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "registered repository project at "+filepath.Join("~", "worktrees", "project", ".git"))
	assert.Contains(t, result.stderr, "imported 2 worktrees:")
	assert.Contains(t, result.stderr, "main: "+fixture.clonePath+
		" -> "+filepath.Join("~", "worktrees", "project", "main", "project"))
	assert.Contains(t, result.stderr, "feature/login: "+featurePath+
		" -> "+filepath.Join("~", "worktrees", "project", "feature/login", "project"))
	assert.Contains(t, result.stderr, "moved to the system trash")
	lines := strings.Split(strings.TrimSpace(result.stderr), "\n")
	require.Len(t, lines, 5)
	assert.Contains(t, lines[0], "registered repository project")
	assert.Contains(t, lines[1], "imported 2 worktrees:")
	assert.Contains(t, lines[2], "main: ")
	assert.Contains(t, lines[3], "feature/login: ")
	assert.Contains(t, lines[4], "moved to the system trash")
}

func TestRepoImportTrashesOldWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	// Old worktrees go through the trash CLI instead of being deleted.
	trashed := fixture.trashedPaths(t)
	assert.Contains(t, trashed, fixture.clonePath)
	assert.Contains(t, trashed, featurePath)
}

func TestRepoImportMovesLinkedWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	mainTarget := fixture.managedWorktreePath("project", "main")
	featureTarget := fixture.managedWorktreePath("project", "feature/login")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")
	fixture.assertWorktreeBranchAt(t, featureTarget, "feature/login")

	_, err := os.Stat(featurePath)
	require.ErrorIs(t, err, os.ErrNotExist)

	listResult := runTimberFromWithRuntime(t, fixture.runtime, fixture.runtime.HomeDirectory, "list", at("project", ""))
	require.NoError(t, listResult.err, listResult.stderr)
	assert.Contains(t, listResult.stdout, "main")
	assert.Contains(t, listResult.stdout, "feature/login")
}

func TestRepoImportRecreatesDetachedWorktree(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	detachedPath := filepath.Join(filepath.Dir(fixture.clonePath), "detached-worktree")
	runGitCommand(t, fixture.clonePath, "worktree", "add", "--detach", detachedPath, "HEAD")

	detachedHash := strings.TrimSpace(runGitCommand(t, detachedPath, "rev-parse", "HEAD"))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	detachedTarget := fixture.managedWorktreePath("project", detachedHash[:7])
	fixture.assertWorktreeBranchAt(t, detachedTarget, "HEAD")
	_, err := os.Stat(detachedTarget)
	require.NoError(t, err)

	assert.Contains(t, result.stderr, "imported 2 worktrees:")

	_, err = os.Stat(detachedPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportSkipsPrunableWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	featurePath := filepath.Join(filepath.Dir(fixture.clonePath), "feature-worktree")
	runGitCommand(t, fixture.clonePath, "branch", "feature/login")
	runGitCommand(t, fixture.clonePath, "worktree", "add", featurePath, "feature/login")
	require.NoError(t, os.RemoveAll(featurePath))

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	// The missing worktree is reported, not silently dropped.
	assert.Contains(t, result.stderr, "skipped worktree")
	assert.Contains(t, result.stderr, featurePath)
	assert.Contains(t, result.stderr, "imported 1 worktrees:")

	// Its branch still survives in the new bare repository.
	runGitCommand(t, fixture.barePath("project"), "show-ref", "--verify", "refs/heads/feature/login")

	mainTarget := fixture.managedWorktreePath("project", "main")
	fixture.assertWorktreeBranchAt(t, mainTarget, "main")
}

func TestRepoImportSkipsWorktreeWithNoCommits(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	// An unborn repository has no commits to move; import registers it and
	// reports the skipped worktree instead of failing.
	unbornPath := filepath.Join(filepath.Dir(fixture.clonePath), "unborn")
	require.NoError(t, os.MkdirAll(unbornPath, 0o755))
	runGitCommand(t, unbornPath, "init")

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", unbornPath)
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "skipped worktree")
	assert.Contains(t, result.stderr, unbornPath)
	assert.Contains(t, result.stderr, "no commits yet")
	assert.Contains(t, result.stderr, "imported 0 worktrees:")

	// The source repository is left in place.
	_, err := os.Stat(unbornPath)
	require.NoError(t, err)
	// The placeholder holds the bare repository and no checkout.
	barePath := fixture.barePath("unborn")
	_, err = os.Stat(barePath)
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Dir(barePath))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, ".git", entries[0].Name())
}

func TestRepoImportPreservesLocalOnlyBranches(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	runGitCommand(t, fixture.clonePath, "branch", "local-only")

	result := fixture.importRun(t, "--name", "project")
	require.NoError(t, result.err, result.stderr)

	runGitCommand(t, fixture.barePath("project"), "show-ref", "--verify", "refs/heads/local-only")
}

func TestRepoImportDerivesNameFromRemote(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	result := fixture.importRun(t)
	require.NoError(t, result.err, result.stderr)

	assert.Contains(t, result.stderr, "registered repository remote at "+filepath.Join("~", "worktrees", "remote", ".git"))
	_, err := os.Stat(fixture.barePath("remote"))
	require.NoError(t, err)
}

func TestRepoImportFailsBeforeMutatingWhenTargetExists(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	targetPath := fixture.managedWorktreePath("project", "main")
	require.NoError(t, os.MkdirAll(targetPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetPath, "README.md"), []byte("occupied\n"), 0o644))

	result := fixture.importRun(t, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")

	// Nothing was touched: source remains, no bare repository was created.
	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
	_, err = os.Stat(fixture.barePath("project"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportFailsWhenRepoNameAlreadyExists(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	require.NoError(t, os.MkdirAll(fixture.barePath("project"), 0o755))

	result := fixture.importRun(t, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already exists")

	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
}

func TestRepoImportFailsWhenAlreadyRegistered(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	require.NoError(t, testRepository.runTimber(t, "create", at(testRepoName, "main")).err)

	result := testRepository.runTimberFrom(
		t,
		testRepository.worktreePath("main"),
		"repo", "import", testRepository.worktreePath("main"),
	)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "already registered")
}

func TestRepoImportFailsOutsideRepository(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)
	emptyPath := filepath.Join(filepath.Dir(fixture.clonePath), "not-a-repo")
	require.NoError(t, os.MkdirAll(emptyPath, 0o755))

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", emptyPath)
	require.Error(t, result.err)
}

func TestRepoImportSuggestsRepoAddForBareRepository(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", fixture.remotePath)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "bare repository")
	assert.Contains(t, result.err.Error(), "timber repo add")
}

func TestRepoImportSuggestsRepoAddForBareRepositoryWithWorktrees(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	// A bare repository with a linked worktree is already in the managed
	// shape; importing it must point at repo add instead.
	base := filepath.Dir(fixture.clonePath)
	barePath := filepath.Join(base, "already-bare.git")
	runGitCommand(t, base, "clone", "--bare", fixture.remotePath, barePath)
	linkedPath := filepath.Join(base, "linked")
	runGitCommand(t, barePath, "worktree", "add", linkedPath, "main")

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", linkedPath)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "bare repository")
	assert.Contains(t, result.err.Error(), "timber repo add")

	// The source worktree is untouched.
	_, err := os.Stat(linkedPath)
	require.NoError(t, err)
}

func TestRepoImportFailsBeforeMutatingWhenTrashCommandMissing(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.TrashExecutable = "timber-missing-trash"
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", fixture.clonePath, "--name", "project")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "trash command")

	// Nothing was touched: source remains, no bare repository was created.
	_, err := os.Stat(fixture.clonePath)
	require.NoError(t, err)
	_, err = os.Stat(fixture.barePath("project"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepoImportFailsOnMissingPath(t *testing.T) {
	t.Parallel()
	fixture := newImportFixture(t)

	runtime := fixture.runtime
	runtime.CurrentDirectory = runtime.HomeDirectory
	missingPath := filepath.Join(filepath.Dir(fixture.clonePath), "missing")
	result := runTimberFromWithRuntime(t, runtime, runtime.HomeDirectory, "repo", "import", missingPath)
	require.Error(t, result.err)
}
