package timber

import (
	"bytes"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/cli/root"
)

func newRootCommand(runtime Runtime) *cobra.Command {
	return root.New(runtime.HomeDirectory, runtime.SkillContent, Commands(runtime)...)
}

func newTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	return newRootCommand(testRuntime(t))
}

func newTestRootCommandWithRuntime(t *testing.T, runtime Runtime) *cobra.Command {
	t.Helper()
	return newRootCommand(runtime)
}

func runCompleteWithRuntime(t *testing.T, runtime Runtime, args ...string) string {
	t.Helper()
	command := newTestRootCommandWithRuntime(t, runtime)
	command.SetArgs(append([]string{"__complete"}, args...))
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(io.Discard)
	require.NoError(t, command.Execute())
	return stdout.String()
}
