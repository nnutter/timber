package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICreateRejectsHerdrAndNoHerdr(t *testing.T) {
	t.Parallel()
	result := runTimberCommand(t, "tui", "--herdr", "--no-herdr")
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "if any flags in the group [herdr no-herdr] are set none of the others can be")
}
