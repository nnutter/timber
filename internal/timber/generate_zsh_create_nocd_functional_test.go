package timber

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalZshCreateHonorsDirectoryChangeOverrides(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		flag       string
		herdr      string
		forwarded  string
		wantChange bool
	}{
		{name: "no-cd", flag: "--no-cd", forwarded: "create\nfeature@repo\n"},
		{name: "explicit herdr", flag: "--herdr", forwarded: "create\n--herdr\nfeature@repo\n"},
		{name: "automatic herdr", herdr: "1", forwarded: "create\nfeature@repo\n"},
		{name: "suppress automatic herdr", flag: "--no-herdr", herdr: "1", forwarded: "create\n--no-herdr\nfeature@repo\n", wantChange: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			target := resolvedTempDir(t)
			command := generatedZshCommand(t, `#!/bin/sh
printf '%s\n' "$TARGET_DIR" > "$TIMBER_CREATE_PATH_FILE"
printf '%s\n' "$@"
`, `source "$1"; shift; t create "$@" feature@repo || exit $?; pwd -P`)
			if testCase.flag != "" {
				command.Args = append(command.Args, testCase.flag)
			}
			command.Env = replaceTestEnvironment(command.Env, "TARGET_DIR="+target, "HERDR_ENV="+testCase.herdr)

			output, err := command.CombinedOutput()

			require.NoError(t, err, string(output))
			wantDirectory := command.Dir
			if testCase.wantChange {
				wantDirectory = target
			}
			assert.Equal(t, testCase.forwarded+wantDirectory, strings.TrimSuffix(string(output), "\n"))
		})
	}
}
