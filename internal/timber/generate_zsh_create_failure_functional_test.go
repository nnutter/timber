package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalZshCreateFailurePreservesDirectoryAndStatus(t *testing.T) {
	t.Parallel()
	command := generatedZshCommand(t, "#!/bin/sh\nexit 17\n",
		`source "$1"; t create feature@repo >/dev/null; result=$?; printf '%s\n' "$result"; pwd -P`)

	output, err := command.CombinedOutput()

	require.NoError(t, err, string(output))
	assert.Equal(t, "17\n"+command.Dir, strings.TrimSpace(string(output)))
}
