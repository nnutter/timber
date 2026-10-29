package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICommandRegistersFlags(t *testing.T) {
	t.Parallel()
	command := New(Dependencies{})

	for _, name := range []string{"herdr", "no-herdr", "no-title"} {
		flag := command.Flags().Lookup(name)
		require.NotNil(t, flag, "flag %q", name)
		assert.Equal(t, "false", flag.DefValue, "flag %q", name)
	}

	assert.Equal(t, "Hide the title header", command.Flags().Lookup("no-title").Usage)
}
