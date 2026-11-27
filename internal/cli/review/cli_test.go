package cli

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/display"
	"github.com/nnutter/timber/internal/testenv"
	"github.com/nnutter/timber/internal/timber/review"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gitTestEnv(t, dir, "", args...)
}

func gitTestEnv(t *testing.T, dir, override string, args ...string) string {
	t.Helper()
	cmd := testenv.GitCommand(t, dir, args...)
	if override != "" {
		cmd.Env = append(cmd.Env, override)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func featureRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "commit", "--allow-empty", "-m", "Base")
	git(t, dir, "checkout", "-b", "feature")
	git(t, dir, "branch", "--set-upstream-to=main")
	git(t, dir, "commit", "--allow-empty", "-m", "Add feature\n\nChange-Id: I"+strings.Repeat("a", 40))
	return dir
}

func execute(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	var output, terminal bytes.Buffer
	err := Execute(t.Context(), args, dir, strings.NewReader(""), &output, &terminal, "test-version")
	return output.String(), terminal.String(), err
}

func TestGitDispatchesReviewSubcommand(t *testing.T) {
	binary := buildGitReview(t)
	dir := featureRepo(t)
	t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, terminal, err := executeBinary(t, "git", dir, "review", "status", "--needs-review", "--json")
	require.NoError(t, err, "%s", terminal)
	require.Empty(t, terminal)
	checkNeedsReviewJSON(t, out)
}

func checkNeedsReviewJSON(t *testing.T, out string) {
	t.Helper()
	require.Equal(t, 1, strings.Count(out, "\n"))
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "status", result.Command)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "needs_review", result.Entries[0].Status)
	require.Len(t, result.Entries[0].Stack.Commits[0], 40)
	require.NotContains(t, out, "\x1b")
}

func TestCommandsShareStructuredResults(t *testing.T) {
	t.Parallel()
	dir := featureRepo(t)
	for _, args := range [][]string{
		{"status", "--needs-review", "--json"},
		{"--json", "status", "--needs-review"},
	} {
		out, terminal, err := execute(t, dir, args...)
		require.NoError(t, err, "command: %v\nstdout: %s\nstderr: %s", args, out, terminal)
		require.Empty(t, terminal)
		checkNeedsReviewJSON(t, out)
	}
	out, terminal, err := execute(t, dir, "status", "--needs-review")
	require.NoError(t, err)
	require.Empty(t, terminal)
	require.Contains(t, out, "Needs review")
	require.Contains(t, out, "Add feature")
	require.Contains(t, out, "COMMITS (inclusive)")
	require.NotContains(t, out, "Approved stacks are omitted from this summary.")
	out, terminal, err = execute(t, dir, "confirm", "HEAD", "--json")
	require.NoError(t, err)
	require.Empty(t, terminal)
	var approved review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &approved))
	require.Equal(t, "approved", approved.Status)
	require.Equal(t, "approve", approved.Command)
	require.Equal(t, "user-command", approved.Entries[0].Attempt.ApprovalSource)
	out, _, err = execute(t, dir, "status", "--needs-review", "--json")
	require.NoError(t, err)
	var listed review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &listed))
	require.Equal(t, "idle", listed.Status)
	require.Empty(t, listed.Entries)
}

func TestJSONErrorsIncludeArgumentAndGitFailures(t *testing.T) {
	t.Parallel()
	dir := featureRepo(t)
	git(t, dir, "branch", "--unset-upstream")
	for _, args := range [][]string{
		{"status", "--needs-review", "--json"},
		{"approve", "--json"},
		{"commits", "--unknown", "--json"},
		{"not-a-command", "--json"},
		{"--json", "status", "--needs-review", "--unknown"},
	} {
		out, terminal, err := execute(t, dir, args...)
		require.Error(t, err)
		require.Empty(t, terminal)
		require.Equal(t, 1, strings.Count(out, "\n"))
		var result display.ErrorResult
		require.NoError(t, json.Unmarshal([]byte(out), &result))
		require.Equal(t, "error", result.Status)
		require.NotEmpty(t, result.Error)
	}
}

func TestReviewBoundsReachAutomaticSelection(t *testing.T) {
	dir := featureRepo(t)
	git(t, dir, "commit", "--allow-empty", "-m", "Second\n\nChange-Id: I"+strings.Repeat("b", 40))
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	require.NoError(t, (review.Store{Path: view.Path}).Update(func(state *review.State) error {
		state.Attempts = append(state.Attempts, review.Attempt{ID: "bounded", Stack: view.Stacks[1], Status: "incomplete", StartedAt: "2026-01-01T00:00:00Z"})
		return nil
	}))
	log := filepath.Join(t.TempDir(), "launch")
	t.Setenv("TEST_LAUNCH_LOG", log)
	installFakeTuicr(t, `printf '%s\n' "$4" >> "$TEST_LAUNCH_LOG"`)
	out, terminal, err := execute(t, dir, "commits", "--start", "HEAD", "--end", "HEAD", "--json")
	require.NoError(t, err)
	require.Empty(t, terminal)
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	// The bounded stack resumes its captured attempt instead of asking.
	require.Equal(t, "cancelled", result.Status)
	launch, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, view.Stacks[1].Base+".."+view.Stacks[1].Tip+"\n", string(launch))
}

func TestReviewAliasResumesTheInterruptedAttempt(t *testing.T) {
	dir := featureRepo(t)
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	store := review.Store{Path: view.Path}
	require.NoError(t, store.Update(func(state *review.State) error {
		state.Attempts = append(state.Attempts, review.Attempt{ID: "interrupted", Stack: view.Stacks[0], Status: "incomplete", StartedAt: "2026-01-01T00:00:00Z"})
		return nil
	}))
	log := filepath.Join(t.TempDir(), "launch")
	t.Setenv("TEST_LAUNCH_LOG", log)
	installFakeTuicr(t, `printf '%s\n' "$4" >> "$TEST_LAUNCH_LOG"`)
	out, terminal, err := execute(t, dir, "challenge", "--json")
	require.NoError(t, err)
	require.Empty(t, terminal)
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "cancelled", result.Status)
	require.Equal(t, "commits", result.Command)
	launch, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, view.Stacks[0].Base+".."+view.Stacks[0].Tip+"\n", string(launch))
}

func TestTerminalRenderingRetainsFeedbackAndRemovesControlSequences(t *testing.T) {
	t.Parallel()
	dir := featureRepo(t)
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	attempt := review.Attempt{ID: "feedback", Stack: view.Stacks[0], Status: "feedback", StartedAt: "2026-01-01T00:00:00Z"}
	attempt.Review = jsontext.Value(`{"id":"saved","version":"1.3","reviewed":1,"files":2,"complete":false,"comments":[{"content":"Revise validation\u001b[31m","location":"line","path":"main.go","stored_line":"12"}],"sessionNotes":"Explain the change"}`)
	require.NoError(t, (review.Store{Path: view.Path}).Update(func(state *review.State) error { state.Attempts = append(state.Attempts, attempt); return nil }))
	out, _, err := execute(t, dir, "status", "--needs-review", "--verbose")
	require.NoError(t, err)
	require.Contains(t, out, "Feedback")
	require.Contains(t, out, "main.go:12: Revise validation")
	require.Contains(t, out, "Session notes: Explain the change")
	require.NotContains(t, out, "\x1b")
}

func TestReviewKeepsTuicrOutputOffJSONStdout(t *testing.T) {
	dir := featureRepo(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "tuicr"), []byte("#!/bin/sh\nprintf 'tuicr terminal output\\n' >&2\nprintf 'exported feedback\\n'\n"), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, terminal, err := execute(t, dir, "commits", "--json")
	require.NoError(t, err)
	require.Equal(t, "tuicr terminal output\n", terminal)
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "cancelled", result.Status)
	require.NotContains(t, out, "tuicr terminal output")
	require.NotContains(t, out, "exported feedback")
}

func TestTuicrKeepsWorktreeLockedAfterGitReviewDies(t *testing.T) {
	dir := featureRepo(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := t.TempDir()
	binary := buildGitReview(t)
	started := filepath.Join(bin, "started")
	exitFile := filepath.Join(bin, "exit")
	t.Setenv("TEST_STARTED", started)
	t.Setenv("TEST_EXIT", exitFile)
	script := "#!/bin/sh\nset -eu\nprintf started > \"$TEST_STARTED\"\nwhile [ ! -f \"$TEST_EXIT\" ]; do sleep 0.05; done\nprintf finished > \"$TEST_STARTED\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "tuicr"), []byte(script), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd := exec.CommandContext(t.Context(), binary, "commits", "--json")
	cmd.Dir = dir
	require.NoError(t, cmd.Start())
	defer func() {
		require.NoError(t, os.WriteFile(exitFile, nil, 0o600))
		require.Eventually(t, func() bool {
			data, err := os.ReadFile(started)
			if err != nil {
				return false
			}
			return string(data) == "finished"
		}, 5*time.Second, 10*time.Millisecond)
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(started); return err == nil }, 30*time.Second, 10*time.Millisecond, "tuicr did not start")
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	out, _, err := execute(t, dir, "commits", "--json")
	require.ErrorContains(t, err, "worktree is in use")
	var result display.ErrorResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "error", result.Status)
}

func TestHelpAndVersion(t *testing.T) {
	t.Parallel()
	out, _, err := execute(t, t.TempDir(), "--help")
	require.NoError(t, err)
	require.Contains(t, out, "approve")
	require.Contains(t, out, "commits")
	require.Contains(t, out, "reset")
	require.Contains(t, out, "files")
	require.Contains(t, out, "--json")
	require.NotContains(t, out, "confirm")
	require.NotContains(t, out, "challenge")
	for _, name := range []string{"needs-review", "review", "review-running", "running"} {
		out, _, err = execute(t, t.TempDir(), name)
		require.ErrorContains(t, err, "unknown command")
		require.Empty(t, out)
	}
	out, _, err = execute(t, t.TempDir(), "status", "--help")
	require.NoError(t, err)
	require.Contains(t, out, "is-reviewing")
	out, _, err = execute(t, t.TempDir(), "--version")
	require.NoError(t, err)
	require.Contains(t, out, "test-version")
}

func TestResetClearsSelectedStacks(t *testing.T) {
	dir := featureRepo(t)
	git(t, dir, "commit", "--allow-empty", "-m", "Second\n\nChange-Id: I"+strings.Repeat("b", 40))
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	store := review.Store{Path: view.Path}
	attempts := func(state *review.State) error {
		state.Attempts = nil
		for i, stack := range view.Stacks {
			state.Attempts = append(state.Attempts, review.Attempt{ID: stack.Key, Stack: stack, Status: "approved", StartedAt: "2026-01-01T00:00:00Z", ApprovalSource: "user-command"})
			_ = i
		}
		return nil
	}
	require.NoError(t, store.Update(attempts))
	out, _, err := execute(t, dir, "reset", "HEAD", "--json")
	require.NoError(t, err)
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "reset", result.Status)
	state, err := store.Load()
	require.NoError(t, err)
	require.Len(t, state.Attempts, 1, "reset clears only the selected stack")
	require.Equal(t, view.Stacks[0].Key, state.Attempts[0].ID)

	require.NoError(t, store.Update(attempts))
	out, _, err = execute(t, dir, "reset", "--json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "reset", result.Status)
	state, err = store.Load()
	require.NoError(t, err)
	require.Empty(t, state.Attempts, "reset without a selector clears the branch")
	out, _, err = execute(t, dir, "status", "--needs-review", "--json")
	require.NoError(t, err)
	var listed review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &listed))
	require.Equal(t, "pending", listed.Status)
}
