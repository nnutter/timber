package timber

import (
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
