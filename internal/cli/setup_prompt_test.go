package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

func TestSetupPromptChoices(t *testing.T) {
	t.Parallel()
	for _, kind := range []timber.SetupKind{timber.SetupMiseTrust, timber.SetupHKInstall} {
		for _, testCase := range []struct {
			name, input string
			want        bool
			retry       bool
		}{
			{name: "yes", input: " YES \n", want: true},
			{name: "no", input: "N\n"},
			{name: "default", input: "\n"},
			{name: "retry", input: "maybe\ny\n", want: true, retry: true},
		} {
			t.Run(string(kind)+"/"+testCase.name, func(t *testing.T) {
				var output bytes.Buffer
				choice, err := (setupPrompter{}).Choose(strings.NewReader(testCase.input), &output, kind, timber.SetupIdentity{Username: "alice"})
				require.NoError(t, err)
				assert.Equal(t, testCase.want, choice)
				assert.Contains(t, output.String(), "GitHub organizations: none\n")
				assert.Equal(t, testCase.retry, strings.Contains(output.String(), "Enter yes or no.\n"))
			})
		}
	}
}

type setupFailWriter struct{ err error }

func (x setupFailWriter) Write([]byte) (int, error) { return 0, x.err }

func TestSetupPromptReturnsInputAndOutputErrors(t *testing.T) {
	t.Parallel()
	for _, kind := range []timber.SetupKind{timber.SetupMiseTrust, timber.SetupHKInstall} {
		t.Run(string(kind), func(t *testing.T) {
			var output bytes.Buffer
			choice, err := (setupPrompter{}).Choose(strings.NewReader("yes"), &output, kind, timber.SetupIdentity{})
			require.ErrorIs(t, err, io.EOF)
			assert.False(t, choice)
			writeErr := errors.New("write failed")
			choice, err = (setupPrompter{}).Choose(strings.NewReader("yes\n"), setupFailWriter{writeErr}, kind, timber.SetupIdentity{})
			require.ErrorIs(t, err, writeErr)
			assert.False(t, choice)
			require.ErrorIs(t, (setupPrompter{}).ReportDisabled(setupFailWriter{writeErr}, kind), writeErr)
		})
	}
}
