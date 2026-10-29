package timber

import (
	"bytes"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/cli/root"
)

func (x testRepository) runTimber(t *testing.T, args ...string) commandResult {
	t.Helper()
	return x.runTimberFrom(t, x.home, args...)
}

func (x testRepository) runTimberFrom(t *testing.T, directory string, args ...string) commandResult {
	t.Helper()

	runtime := x.runtime
	runtime.CurrentDirectory = directory
	return runTimberCommandWithRuntime(t, runtime, args...)
}

func runTimberFromWithRuntime(t *testing.T, runtime Runtime, directory string, args ...string) commandResult {
	t.Helper()

	runtime.CurrentDirectory = directory
	return runTimberCommandWithRuntime(t, runtime, args...)
}

func runTimberCommand(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runTimberCommandWithRuntime(t, testRuntime(t), args...)
}

func runTimberCommandWithRuntime(t *testing.T, runtime Runtime, args ...string) commandResult {
	t.Helper()

	command := newTestRootCommandWithRuntime(t, runtime)
	command.SetArgs(args)
	command.SetIn(bytes.NewBuffer(nil))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	err := command.Execute()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func newRootCommand(runtime Runtime) *cobra.Command {
	return root.New(runtime.HomeDirectory, runtime.SkillContent, Commands(runtime)...)
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
