package bump

import (
	"bytes"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	versionbump "github.com/nnutter/timber/internal/timber/bump"
)

func newTestCommand(bump func(io.Reader, string, Request) (string, error)) *cobra.Command {
	command := New(Dependencies{
		Bump: bump,
		Complete: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	})
	command.SetIn(bytes.NewReader(nil))
	return command
}

func TestBumpPassesFlagsAndPrintsTag(t *testing.T) {
	var raw string
	var request Request
	command := newTestCommand(func(_ io.Reader, selector string, given Request) (string, error) {
		raw = selector
		request = given
		return "v1.2.4", nil
	})
	command.SetArgs([]string{"release@owner/timber", "--patch", "--pattern", "v1.*", "--no-push"})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})

	require.NoError(t, command.Execute())
	assert.Equal(t, "v1.2.4\n", stdout.String())
	assert.Equal(t, "release@owner/timber", raw)
	assert.Equal(t, Request{Kind: versionbump.Patch, Pattern: "v1.*", NoPush: true}, request)
}

func TestBumpDefaultsToTheCurrentWorktree(t *testing.T) {
	raw := "unset"
	command := newTestCommand(func(_ io.Reader, selector string, _ Request) (string, error) {
		raw = selector
		return "v2.0.0", nil
	})
	command.SetArgs([]string{"--major"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	require.NoError(t, command.Execute())
	assert.Empty(t, raw)
}

func TestBumpRequiresExactlyOneKind(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "none"},
		{name: "two", args: []string{"--major", "--patch"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := newTestCommand(func(io.Reader, string, Request) (string, error) {
				require.Fail(t, "bump must not run without exactly one kind")
				return "", nil
			})
			command.SetArgs(tt.args)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})

			require.ErrorContains(t, command.Execute(), "exactly one of --major, --minor, or --patch is required")
		})
	}
}

func TestBumpReportsBumpErrors(t *testing.T) {
	command := newTestCommand(func(io.Reader, string, Request) (string, error) {
		return "", assert.AnError
	})
	command.SetArgs([]string{"--minor"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	require.ErrorIs(t, command.Execute(), assert.AnError)
}
