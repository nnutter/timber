package todo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditorCommandLineFailsWithoutEditorOrFallback(t *testing.T) {
	t.Parallel()
	_, err := editorCommandLine([]string{"EDITOR=", "PATH=" + t.TempDir()})
	require.ErrorContains(t, err, "EDITOR is not set")
}
