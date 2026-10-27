package timber

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedZshReviewAndCompletion(t *testing.T) {
	t.Parallel()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	repository := newTestRepository(t)
	const branch = "feature/review-shell"
	require.NoError(t, repository.runTimber(t, "create", at(testRepoName, branch)).err)
	repository.commitFileInWorktree(t, branch, "review.txt", "review me\n")
	bin := buildTestTimberBinary(t)
	// The built-in commands must not delegate to an installed git-review.
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git-review"), []byte("#!/bin/sh\nexit 91\n"), 0o755))
	outDir := resolvedTempDir(t)
	require.NoError(t, repository.runTimber(t, "generate", "zsh", "--out", outDir).err)

	run := func(program string, args ...string) commandResult {
		t.Helper()
		command := testCommand(t, zsh, append([]string{"-f", "-c", program, "--", outDir}, args...)...)
		command.Dir = repository.worktreePath(branch)
		command.Env = replaceTestEnvironment(repository.runtime.Environment,
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GIT_PAGER=cat",
		)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}
	for _, args := range [][]string{{"review", "status", "--json"}, {"review", "--json", "status"}} {
		result := run(`source "$1/t"; shift; t "$@"`, args...)
		require.NoError(t, result.err, result.stderr)
		assert.NotContains(t, result.stderr, "ERROR")
		var status struct {
			Version int    `json:"version"`
			Command string `json:"command"`
			Summary []struct {
				State string `json:"state"`
				Count int    `json:"count"`
			} `json:"summary"`
		}
		require.NoError(t, json.Unmarshal([]byte(result.stdout), &status))
		assert.Equal(t, 1, status.Version)
		assert.Equal(t, "status", status.Command)
		require.NotEmpty(t, status.Summary)
		assert.Equal(t, "needs_review", status.Summary[0].State)
		assert.Equal(t, 1, status.Summary[0].Count)
	}
	log := run(`source "$1/t"; t review log --oneline`)
	require.NoError(t, log.err, log.stderr)
	assert.Contains(t, log.stdout, "change")

	for _, testCase := range []struct {
		args []string
		want string
	}{
		{args: []string{"re"}, want: "review:"},
		{args: []string{"review", "sta"}, want: "status:"},
		{args: []string{"review", "lo"}, want: "log:"},
		{args: []string{"review", "status", "--needs-"}, want: "--needs-review:"},
		{args: []string{"review", "log", "--one"}, want: "--oneline:"},
		{args: []string{"review", "status", "--js"}, want: "--json:"},
	} {
		result := run(`source "$1/_t"
shift
words=(t "$@")
CURRENT=${#words}
_describe() { print -rl -- "${completions[@]}"; }
_t`, testCase.args...)
		require.NoError(t, result.err, result.stderr)
		assert.Contains(t, result.stdout, testCase.want, "args=%v", testCase.args)
	}
	failed := run(`source "$1/t"; t review status --unknown --json`)
	require.Error(t, failed.err)
	assert.Empty(t, failed.stderr)
	assert.True(t, strings.HasPrefix(failed.stdout, "{"), failed.stdout)
	var failure struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(failed.stdout), &failure))
	assert.Equal(t, "error", failure.Status)
	assert.Contains(t, failure.Error, "unknown")
}
