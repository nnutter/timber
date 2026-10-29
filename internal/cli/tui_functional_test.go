package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICreateRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	result := repository.runTimber(t, "tui")
	require.ErrorContains(t, result.err, "tui requires an interactive terminal")
}

func TestTUICreateRejectsHerdrAndNoHerdr(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "tui", "--herdr", "--no-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}
