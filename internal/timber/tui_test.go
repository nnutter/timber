package timber

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICreateNoTitlePassesShowTitleFalse(t *testing.T) {
	t.Parallel()
	testRepository := newTestRepository(t)
	prompter := &stubCreateWizardPrompter{selection: WizardSelection{Cancelled: true}}
	options := &tuiTestOptions{runtime: testRepository.runtime, noTitle: true}
	result := runTUICreate(t, options, prompter)

	require.NoError(t, result.err, result.stderr)
	assert.False(t, prompter.showTitle)
}

func TestCreateWizardRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	prompter := bubbleteaCreateWizardPrompter{interactive: func() bool { return false }}
	_, err := prompter.Prompt(bytes.NewBuffer(nil), io.Discard, []RepoRef{{Name: testRepoName}}, make([]WorktreeRef, 0), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
}
