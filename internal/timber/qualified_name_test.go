package timber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectAtInWorktreeName(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateWorktreeName("feature/login"))
	require.NoError(t, ValidateWorktreeName(""))
	err := ValidateWorktreeName("foo@bar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain @")
}
