package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckReportsInvalidCommitsInTheSelectedRange(t *testing.T) {
	t.Parallel()
	runtime := testRuntime(t)
	repository := resolvedTempDir(t)
	runGitCommand(t, repository, "init", "-b", "main")
	runGitCommand(t, repository, "commit", "--allow-empty", "-m", "Not part of the range")
	runGitCommand(t, repository, "branch", "base")
	runGitCommand(t, repository, "branch", "--set-upstream-to", "base")
	const valid = "Add a valid commit\n\nChange-Id: I0000000000000000000000000000000000000000"
	runGitCommand(t, repository, "commit", "--allow-empty", "-m", valid)
	result := runTimberFromWithRuntime(t, runtime, repository, "check")
	require.NoError(t, result.err, result.stderr)

	for _, subject := range []string{"fixup! Add a valid commit", "amend! Update the commit", "reword! Rename the commit", "TODO review this.", "WIP: update", "Temp: " + strings.Repeat("a", 95)} {
		runGitCommand(t, repository, "commit", "--allow-empty", "-m", subject, "-m", strings.Repeat("b", 73))
	}
	result = runTimberFromWithRuntime(t, runtime, repository, "check", "base..HEAD")
	require.Error(t, result.err)
	message := result.err.Error()
	for _, expected := range []string{"fixup!", "amend!", "reword!", "TODO", "WIP:", "Temp:", "unfinished", "Change-Id"} {
		assert.Contains(t, message, expected)
	}
	for _, format := range []string{"capital letter", "period", "100 characters", "72 characters"} {
		assert.NotContains(t, message, format)
	}
	assert.NotContains(t, message, "Not part of the range")

	runGitCommand(t, repository, "commit", "--allow-empty", "-m", "lowercase "+strings.Repeat("a", 95)+".",
		"-m", strings.Repeat("b", 73), "-m", "Change-Id: I1111111111111111111111111111111111111111")
	result = runTimberFromWithRuntime(t, runtime, repository, "check", "HEAD^..HEAD")
	require.Error(t, result.err)
	for _, format := range []string{"capital letter", "period", "100 characters", "72 characters"} {
		assert.Contains(t, result.err.Error(), format)
	}
}
