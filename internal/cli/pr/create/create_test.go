package create

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePassesFlagsAndReportsURL(t *testing.T) {
	var raw string
	var draft Draft
	command := New(Dependencies{
		Create: func(_ context.Context, _ io.Reader, selector string, given Draft) (string, error) {
			raw = selector
			draft = given
			return "https://github.com/owner/timber/pull/7", nil
		},
		Complete: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RenderStatus: func(message string) string { return message },
	})
	command.SetArgs([]string{"feature/pull@owner/timber", "--title", "Add login", "--body", "Body text", "--base", "develop", "--draft"})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)

	require.NoError(t, command.Execute())
	assert.Equal(t, "https://github.com/owner/timber/pull/7\n", stdout.String())
	assert.Equal(t, "created pull request https://github.com/owner/timber/pull/7\n", stderr.String())
	assert.Equal(t, "feature/pull@owner/timber", raw)
	assert.Equal(t, Draft{Title: "Add login", Body: "Body text", Base: "develop", Draft: true}, draft)
}

func TestCreateRequiresTitle(t *testing.T) {
	command := New(Dependencies{
		Create: func(context.Context, io.Reader, string, Draft) (string, error) {
			require.Fail(t, "create must not run without a title")
			return "", nil
		},
		Complete: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RenderStatus: func(message string) string { return message },
	})
	command.SetArgs([]string{"feature/pull@owner/timber"})

	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}
