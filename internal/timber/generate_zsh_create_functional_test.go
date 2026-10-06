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
	for _, custom := range []bool{false, true} {
		name := "default"
		if custom {
			name = "custom"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			target := filepath.Join(resolvedTempDir(t), "worktree with spaces")
			require.NoError(t, os.Mkdir(target, 0o755))
			invocation := `source "$1"; t create feature@repo >/dev/null || exit $?; pwd -P`
			if custom {
				invocation = `source "$1"; t create feature@repo "$TARGET_DIR" >/dev/null || exit $?; pwd -P`
			}
			command := generatedZshCommand(t, `#!/bin/sh
[ "$1" = create ] && [ "$2" = feature@repo ] || exit 98
if [ "$CUSTOM_PATH" = true ]; then
    [ "$#" = 3 ] && [ "$3" = "$TARGET_DIR" ] || exit 97
else
    [ "$#" = 2 ] || exit 96
fi
printf '%s\n' "$TARGET_DIR" > "$TIMBER_CREATE_PATH_FILE"
`, invocation)
			customValue := "false"
			if custom {
				customValue = "true"
			}
			command.Env = replaceTestEnvironment(command.Env, "TARGET_DIR="+target, "CUSTOM_PATH="+customValue)

			output, err := command.CombinedOutput()

			require.NoError(t, err, string(output))
			assert.Equal(t, target, strings.TrimSpace(string(output)))
		})
	}
}
