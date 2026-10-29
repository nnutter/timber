package timber

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTUIRequiresWizardPrompter(t *testing.T) {
	t.Parallel()
	err := testRuntime(t).RunTUI(t.Context(), nil, io.Discard, io.Discard, false, false, false, nil, CreateReporters{})
	require.ErrorContains(t, err, "tui requires a wizard prompter")
}
