package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errImportOutput = errors.New("import output failed")

type failingImportWriter struct {
	bytes.Buffer
	writes int
	failAt int
}

func (w *failingImportWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, errImportOutput
	}
	return w.Buffer.Write(p)
}

func TestRepoImportReportingWriterFailurePreservesProgress(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		failAt int
	}{
		{name: "registration", failAt: 1},
		{name: "summary move", failAt: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newImportFixture(t)
			writer := &failingImportWriter{failAt: tc.failAt}
			command := newRootCommand(fixture.runtime)
			command.SetArgs([]string{"repo", "import", fixture.clonePath, "--name", "project"})
			command.SetOut(io.Discard)
			command.SetErr(writer)

			err := command.Execute()
			require.ErrorIs(t, err, errImportOutput)
			assert.DirExists(t, fixture.barePath("project"))
			if tc.failAt == 1 {
				assert.DirExists(t, fixture.clonePath)
				assert.NoDirExists(t, fixture.managedWorktreePath("project", "main"))
				assert.Empty(t, writer.String())
				assert.Empty(t, fixture.trashedPaths(t))
			} else {
				assert.NoDirExists(t, fixture.clonePath)
				fixture.assertWorktreeBranchAt(t, fixture.managedWorktreePath("project", "main"), "main")
				assert.Contains(t, fixture.trashedPaths(t), fixture.clonePath)
				lines := strings.Split(strings.TrimSpace(writer.String()), "\n")
				require.Len(t, lines, 2)
				assert.Contains(t, lines[0], "registered repository project")
				assert.Contains(t, lines[1], "imported 1 worktrees:")
			}
		})
	}
}
