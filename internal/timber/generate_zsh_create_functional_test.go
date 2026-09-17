package timber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalZshCreateChangesDirectoryToReportedPath(t *testing.T) {
	t.Parallel()
	target := filepath.Join(resolvedTempDir(t), "worktree with spaces")
	require.NoError(t, os.Mkdir(target, 0o755))
	command := generatedZshCommand(t, `#!/bin/sh
[ "$#" = 2 ] && [ "$1" = create ] && [ "$2" = feature@repo ] || exit 98
printf '%s\n' "$TARGET_DIR" > "$TIMBER_CREATE_PATH_FILE"
`, `source "$1"; t create feature@repo >/dev/null || exit $?; pwd -P`)
	command.Env = replaceTestEnvironment(command.Env, "TARGET_DIR="+target)

	output, err := command.CombinedOutput()

	require.NoError(t, err, string(output))
	assert.Equal(t, target, strings.TrimSpace(string(output)))
}
