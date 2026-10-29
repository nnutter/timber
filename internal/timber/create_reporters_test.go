package timber

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failCreateStatusWriter struct {
	writes int
	failAt int
	err    error
}

func (x *failCreateStatusWriter) Write(data []byte) (int, error) {
	x.writes++
	if x.writes == x.failAt {
		return 0, x.err
	}
	return len(data), nil
}

func TestCreateStopsWhenStatusWriteFails(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name   string
		failAt int
	}{
		{name: "created", failAt: 1},
		{name: "herdr opened", failAt: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			repo := newTestRepository(t)
			const branch = "feature/status-error"
			logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
			repo.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
			repo.runtime.GhExecutable = installFakeGh(t)
			writeErr := errors.New("write failed")
			output := &failCreateStatusWriter{failAt: testCase.failAt, err: writeErr}
			reporters := CreateReporters{
				Created: func(output io.Writer, path string) error {
					_, err := fmt.Fprintln(output, "created "+path)
					return err
				},
				HerdrOpened: func(output io.Writer, name string) error {
					_, err := fmt.Fprintln(output, "opened herdr space for "+name)
					return err
				},
			}
			_, _, err := repo.runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, output, "", at(testRepoName, branch), "", false, "", true, false, reporters)
			require.ErrorIs(t, err, writeErr)
			repo.assertPathPresent(t, repo.worktreePath(branch))
			if testCase.failAt == 1 {
				assert.NoFileExists(t, logPath)
			} else {
				assert.NotEmpty(t, readFakeHerdrLog(t, logPath))
			}
		})
	}
}

func TestWizardOpenReturnsHerdrStatusError(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	const branch = "feature/open-status-error"
	_, _, err := repo.runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(testRepoName, branch), "", false, "", false, false, testCreateReporters())
	require.NoError(t, err)
	logPath := filepath.Join(resolvedTempDir(t), "herdr.log")
	repo.runtime.HerdrExecutable = installFakeHerdrSpace(t, logPath)
	repo.runtime.GhExecutable = installFakeGh(t)
	writeErr := errors.New("write failed")
	output := &failCreateStatusWriter{failAt: 1, err: writeErr}
	reporters := testCreateReporters()
	prompter := &stubCreateWizardPrompter{selection: WizardSelection{Action: WizardActionOpen, RepoName: testRepoName, WorktreeName: branch}}
	var stdout bytes.Buffer
	err = repo.runtime.RunTUI(t.Context(), &bytes.Buffer{}, output, &stdout, false, false, false, prompter, reporters)
	require.ErrorIs(t, err, writeErr)
	assert.NotEmpty(t, readFakeHerdrLog(t, logPath))
	assert.Empty(t, stdout.String())
}

func TestCreateRequiresReportersBeforeCreatingWorktree(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	const branch = "feature/no-reporter"
	_, _, err := repo.runtime.CreateManagedWorktree(t.Context(), nil, io.Discard, "", at(testRepoName, branch), "", false, "", false, false, CreateReporters{})
	require.ErrorContains(t, err, "create requires a worktree creation reporter")
	repo.assertPathMissing(t, repo.worktreePath(branch))
}
