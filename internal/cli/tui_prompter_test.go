package cli

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

func TestCreateWizardRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	prompter := bubbleteaCreateWizardPrompter{}
	_, err := prompter.Prompt(bytes.NewBuffer(nil), io.Discard, []timber.RepoRef{{Name: testRepoName}}, make([]timber.WorktreeRef, 0), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
}
