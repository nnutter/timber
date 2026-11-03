package bump

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	versionbump "github.com/nnutter/timber/internal/timber/bump"
)

func newTestCommand(bump func(context.Context, io.Reader, io.Writer, string, Request) (Result, error)) *cobra.Command {
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
	command := newTestCommand(func(_ context.Context, _ io.Reader, _ io.Writer, selector string, given Request) (Result, error) {
		raw = selector
		request = given
		return Result{Tag: "v1.2.4"}, nil
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

func TestBumpPrintsTagAndReleaseURL(t *testing.T) {
	command := newTestCommand(func(context.Context, io.Reader, io.Writer, string, Request) (Result, error) {
		return Result{Tag: "v1.2.4", ReleaseURL: "https://github.com/owner/timber/releases/tag/v1.2.4"}, nil
	})
	command.SetArgs([]string{"--patch", "--release", "--no-open"})
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})

	require.NoError(t, command.Execute())
	assert.Equal(t, "v1.2.4\nhttps://github.com/owner/timber/releases/tag/v1.2.4\n", stdout.String())
}

func TestBumpPassesReleaseFlags(t *testing.T) {
	var request Request
	command := newTestCommand(func(_ context.Context, _ io.Reader, _ io.Writer, _ string, given Request) (Result, error) {
		request = given
		return Result{Tag: "v1.2.4"}, nil
	})
	command.SetArgs([]string{"--minor", "--release", "--no-open"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	require.NoError(t, command.Execute())
	assert.Equal(t, Request{Kind: versionbump.Minor, Release: true, NoOpen: true}, request)
}

func TestBumpDefaultsToTheCurrentWorktree(t *testing.T) {
	raw := "unset"
	command := newTestCommand(func(_ context.Context, _ io.Reader, _ io.Writer, selector string, _ Request) (Result, error) {
		raw = selector
		return Result{Tag: "v2.0.0"}, nil
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
			command := newTestCommand(func(context.Context, io.Reader, io.Writer, string, Request) (Result, error) {
				require.Fail(t, "bump must not run without exactly one kind")
				return Result{}, nil
			})
			command.SetArgs(tt.args)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})

			require.ErrorContains(t, command.Execute(), "exactly one of --major, --minor, or --patch is required")
		})
	}
}

func TestBumpReportsBumpErrors(t *testing.T) {
	command := newTestCommand(func(context.Context, io.Reader, io.Writer, string, Request) (Result, error) {
		return Result{}, assert.AnError
	})
	command.SetArgs([]string{"--minor"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	require.ErrorIs(t, command.Execute(), assert.AnError)
}
