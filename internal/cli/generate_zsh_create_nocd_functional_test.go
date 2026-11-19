package cli

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
		subcommand string
		flag       string
		herdr      string
		forwarded  string
		wantChange bool
	}{
		{name: "no-cd", subcommand: "create", flag: "--no-cd", forwarded: "create\nfeature@repo\n"},
		{name: "alias no-cd", subcommand: "c", flag: "--no-cd", forwarded: "create\nfeature@repo\n"},
		{name: "explicit herdr", subcommand: "create", flag: "--herdr", forwarded: "create\n--herdr\nfeature@repo\n"},
		{name: "automatic herdr", subcommand: "create", herdr: "1", forwarded: "create\nfeature@repo\n"},
		{name: "suppress automatic herdr", subcommand: "create", flag: "--no-herdr", herdr: "1", forwarded: "create\n--no-herdr\nfeature@repo\n", wantChange: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			target := resolvedTempDir(t)
			command := generatedZshCommand(t, `#!/bin/sh
if [ -n "${TIMBER_CREATE_PATH_FILE:-}" ]; then
    printf '%s\n' "$TARGET_DIR" > "$TIMBER_CREATE_PATH_FILE"
fi
printf '%s\n' "$@"
`, `source "$1"; shift; t `+testCase.subcommand+` "$@" feature@repo || exit $?; pwd -P`)
			if testCase.flag != "" {
				command.Args = append(command.Args, testCase.flag)
			}
			command.Env = shellTestEnvironment(command.Env, "TARGET_DIR="+target, "HERDR_ENV="+testCase.herdr)

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
