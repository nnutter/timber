// Package testenv provides process isolation for review test suites.
// It is imported only by test files, never by the command or review library.
package testenv

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/nnutter/gittest"
)

// GitCommand builds a fixture command without inherited Git settings or config.
func GitCommand(t testing.TB, dir string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := gittest.Command(t, dir, args...)
	cmd.Env = GitEnvironment()
	return cmd
}

// GitEnvironment isolates commands that run Git indirectly.
func GitEnvironment() []string {
	return gittest.IsolatedEnviron()
}

// Run clears inherited Git settings before tests start, including parallel tests.
// In-process review calls and child processes then share the isolated environment.
func Run(m *testing.M) {
	if err := isolateGit(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func isolateGit() error {
	// Repository resolves Git through the process PATH. Fix that once, before any
	// tests start; fixture commands and child environments are isolated separately.
	sanitized := gittest.WithEnviron()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		// This is a test-suite opt-in, not a Git repository setting.
		if key == "GIT_REVIEW_REAL_TUICR" {
			continue
		}
		if strings.HasPrefix(key, "GIT_") {
			if err := os.Unsetenv(key); err != nil {
				return err
			}
		}
	}
	for _, entry := range sanitized {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if key == "PATH" {
			if err := os.Setenv("PATH", value); err != nil {
				return err
			}
		}
	}
	// Identity and other repository settings are supplied by each local fixture.
	return errors.Join(os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull), os.Setenv("GIT_CONFIG_NOSYSTEM", "1"))
}
