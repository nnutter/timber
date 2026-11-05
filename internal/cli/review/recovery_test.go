package cli

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/cli/review/display"
	"github.com/nnutter/timber/internal/timber/review"
)

func TestRecoveryDecisionExplainsSavedWork(t *testing.T) {
	for _, tc := range []struct {
		name        string
		saved       jsontext.Value
		want        string
		replacement string
	}{
		{"saved progress", jsontext.Value(`{"reviewed":1,"files":2,"comments":[{"content":"Fix this"}],"sessionNotes":"Check this"}`), "Saved progress: 1/2 files reviewed · Comments: 1 · session notes saved", "changed"},
		{"missing progress", nil, "Saved progress: unavailable", "same"},
		{"missing counters", jsontext.Value(`{"comments":[]}`), "Saved progress: unavailable", "same"},
		{"no replacement", nil, "Saved progress: unavailable", "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saved := review.Stack{Base: "base", Tip: "bbbbbbbbbbbb", Commits: []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb"}, Subject: "Fix recovery\x1b[31m\nhandling"}
			current := review.Stack{Base: "base", Tip: "cccccccccccc", Commits: []string{"cccccccccccc"}, Subject: "Revised recovery"}
			result := review.Result{Command: "commits", Status: "decision_required", Decision: &review.Decision{
				AttemptID:   "internal-id",
				Attempt:     review.Attempt{Stack: saved, Status: "incomplete", StartedAt: "2026-10-16T12:00:00Z", Review: tc.saved},
				Replacement: &current,
			}}
			switch tc.replacement {
			case "same":
				result.Decision.Replacement = &saved
			case "none":
				result.Decision.Replacement = nil
			}
			text, err := display.RenderResult(result, false)
			require.NoError(t, err)
			require.Contains(t, text, "Saved commits (inclusive): aaaaaaaa..bbbbbbbb — Fix recovery handling")
			require.Contains(t, text, "Started: 2026-10-16T12:00:00Z · Status: Incomplete")
			require.Contains(t, text, tc.want)
			if tc.replacement == "changed" {
				require.Contains(t, text, "New review commits (inclusive): cccccccc — Revised recovery")
			} else {
				require.NotContains(t, text, "New review commits")
			}
			if tc.replacement == "none" {
				require.Contains(t, text, "No current replacement scope. Select one with timber review commits <commit>.")
			}
			require.Contains(t, text, "Replacement keeps the prior attempt in history. Tuicr may reuse its saved session.")
			require.NotContains(t, text, "internal-id")
		})
	}
}

// installFakeTuicr puts a scripted tuicr on PATH so a review can run without a
// terminal.
func installFakeTuicr(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tuicr"), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// An interrupted attempt whose captured commits a rewrite removed leaves no
// range that a review can reopen or replace. The review must start on the
// current stacks instead of asking for an impossible choice.
func TestReviewDiscardsAttemptWhoseStackNoLongerExists(t *testing.T) {
	dir := featureRepo(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	installFakeTuicr(t, "exit 0")
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	interrupted := review.Attempt{ID: "interrupted", Stack: view.Stacks[0], Status: "reviewing", StartedAt: "2026-01-01T00:00:00Z"}
	store := review.Store{Path: view.Path}
	require.NoError(t, store.Update(func(state *review.State) error {
		state.Attempts = append(state.Attempts, interrupted)
		return nil
	}))
	// The rewrite keeps the patch but leaves no lineage to the captured commits.
	git(t, dir, "commit", "--amend", "--allow-empty", "-m", "Add feature again")
	out, _, err := execute(t, dir, "commits", "--json")
	require.NoError(t, err)
	var result review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.NotEqual(t, "decision_required", result.Status)
	require.Nil(t, result.Decision)
	state, err := store.Load()
	require.NoError(t, err)
	require.NotContains(t, attemptIDs(state), interrupted.ID)
}

func TestApprovalIgnoresAttemptWhoseStackNoLongerExists(t *testing.T) {
	dir := featureRepo(t)
	view, err := (review.Repository{Dir: dir}).Snapshot(t.Context())
	require.NoError(t, err)
	interrupted := review.Attempt{ID: "interrupted", Stack: view.Stacks[0], Status: "reviewing", StartedAt: "2026-01-01T00:00:00Z"}
	require.NoError(t, (review.Store{Path: view.Path}).Update(func(state *review.State) error {
		state.Attempts = append(state.Attempts, interrupted)
		return nil
	}))
	git(t, dir, "commit", "--amend", "--allow-empty", "-m", "Add feature again")
	out, _, err := execute(t, dir, "approve", "HEAD", "--json")
	require.NoError(t, err)
	var approved review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &approved))
	require.Equal(t, "approved", approved.Status)
	out, _, err = execute(t, dir, "status", "--needs-review", "--json")
	require.NoError(t, err)
	var listed review.Result
	require.NoError(t, json.Unmarshal([]byte(out), &listed))
	require.Equal(t, "idle", listed.Status)
	require.Empty(t, listed.Entries)
	require.Empty(t, listed.Active)
}

func attemptIDs(state review.State) []string {
	ids := make([]string, 0, len(state.Attempts))
	for _, attempt := range state.Attempts {
		ids = append(ids, attempt.ID)
	}
	return ids
}
