package review

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// carryFixture writes a saved session for the current stack with one of its two
// files reviewed and one line comment.
func carryFixture(t *testing.T, reviewer Reviewer, view Snapshot, store string) {
	t.Helper()
	scope := view.Stacks[0]
	path, err := WorktreePath(scope.Repo)
	require.NoError(t, err)
	require.NoError(t, prepareWorktree(t.Context(), reviewer.Service.Repository, path, scope))
	scope.Repo = path
	session := savedFixture(t, scope, store, false)
	short, err := shortSHA(t.Context(), reviewer.Service.Repository, view.Stacks[0].Tip)
	require.NoError(t, err)
	files := session["files"].(map[string]any)
	message := files["CommitMessage"].(map[string]any)
	delete(files, "CommitMessage")
	name := commitMessagePrefix + short + ")"
	message["path"] = name
	message["reviewed"] = true
	files[name] = message
	file := files["main.go"].(map[string]any)
	file["reviewed"] = true
	file["line_comments"] = map[string]any{"1": []any{map[string]any{
		"id": "comment", "content": "Handle the empty case", "comment_type": "issue",
		"created_at": "2026-01-01T00:00:00Z", "author": "user", "lifecycle_state": "local_draft",
	}}}
	writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		attempt := newAttempt(view.Stacks[0], "incomplete")
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
}

func carryRepository(t *testing.T) (Reviewer, Snapshot) {
	t.Helper()
	reviewer, _, _ := testReviewer(t)
	dir := reviewer.Service.Repository.Dir
	gitTest(t, dir, "reset", "--hard", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600))
	gitTest(t, dir, "add", ".")
	gitTest(t, dir, "commit", "-m", "Feature")
	view, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)
	return reviewer, view
}

func TestCarrySavedReviewKeepsMarksAcrossRewrite(t *testing.T) {
	for _, tc := range []struct {
		name         string
		message      string
		wantMessage  bool
		wantReviewed int
	}{
		{name: "reworded", message: "Feature reworded", wantMessage: false, wantReviewed: 1},
		{name: "rebased", message: "", wantMessage: true, wantReviewed: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reviewer, view := carryRepository(t)
			store := t.TempDir()
			reviewer.Reader = SavedReader{Stores: []string{store}}
			carryFixture(t, reviewer, view, store)
			args := []string{"commit", "--amend", "--allow-empty", "--date", "2026-01-01T00:00:00Z"}
			if tc.message == "" {
				args = append(args, "--no-edit")
			} else {
				args = append(args, "-m", tc.message)
			}
			gitTest(t, reviewer.Service.Repository.Dir, args...)
			carried, err := reviewer.Service.Repository.Snapshot(t.Context())
			require.NoError(t, err)
			require.NotEqual(t, view.Stacks[0].Commits, carried.Stacks[0].Commits)
			require.Equal(t, view.Stacks[0].PatchHash, carried.Stacks[0].PatchHash)

			carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

			scope := carried.Stacks[0]
			checkout, err := WorktreePath(scope.Repo)
			require.NoError(t, err)
			scope.Repo = checkout
			saved, err := (SavedReader{Stores: []string{store}}).Read(scope, "")
			require.NoError(t, err)
			require.NotNil(t, saved, "the review moved onto the rewritten commits")
			require.Equal(t, tc.wantReviewed, saved.Reviewed)
			require.Equal(t, 2, saved.Files)
			require.Equal(t, "feedback", saved.Status())

			document, err := readObject(saved.Path)
			require.NoError(t, err)
			files, err := decodeRequired[object](document, "files")
			require.NoError(t, err)
			short, err := shortSHA(t.Context(), reviewer.Service.Repository, carried.Stacks[0].Tip)
			require.NoError(t, err)
			message, err := decodeRequired[object](files, commitMessagePrefix+short+")")
			require.NoError(t, err)
			reviewed, err := decodeRequired[bool](message, "reviewed")
			require.NoError(t, err)
			require.Equal(t, tc.wantMessage, reviewed, "only a changed message asks again")
			main, err := decodeRequired[object](files, "main.go")
			require.NoError(t, err)
			marked, err := decodeRequired[bool](main, "reviewed")
			require.NoError(t, err)
			require.True(t, marked)

			index, entries, err := loadIndex(store)
			require.NoError(t, err)
			require.NotNil(t, index)
			require.NotContains(t, entries, "fixture")
			require.Len(t, entries, 1)
		})
	}
}

func TestCarrySavedReviewLeavesAMatchingRangeAlone(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	before, err := os.ReadFile(filepath.Join(store, "sessions/0123456789abcdef.json"))
	require.NoError(t, err)

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	after, err := os.ReadFile(filepath.Join(store, "sessions/0123456789abcdef.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a current range keeps its session")
	var indexed map[string]any
	raw, err := os.ReadFile(filepath.Join(store, "index.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &indexed))
	require.Contains(t, indexed["entries"], "fixture")
}

// A review that a rewrite moved records the carried progress, so the result and
// the session that tuicr shows describe the same work.
func TestReviewRecordsCarriedProgress(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	gitTest(t, reviewer.Service.Repository.Dir, "commit", "--amend", "--allow-empty", "--date", "2026-01-01T00:00:00Z", "-m", "Feature reworded")
	fakeTuicr(t, "exit 0")

	result, err := reviewer.Review(t.Context(), ReviewOptions{Choice: "replace"})
	require.NoError(t, err)
	require.Equal(t, "feedback", result.Status)
	require.Len(t, result.Entries, 1)
	var saved SavedReview
	require.NoError(t, json.Unmarshal(result.Entries[0].Attempt.Review, &saved))
	require.Equal(t, 1, saved.Reviewed)
	require.Equal(t, 2, saved.Files)
	require.Len(t, saved.Comments, 1)
}

// A store is often reached through a symlinked path, as macOS does for /var, so
// the path that the reader reports differs from the path that names the store.
// The move must still find the index entry.
func TestCarrySavedReviewThroughAliasedStore(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(store, alias))
	reviewer.Reader = SavedReader{Stores: []string{alias}}
	carryFixture(t, reviewer, view, store)
	gitTest(t, reviewer.Service.Repository.Dir, "commit", "--amend", "--allow-empty", "--date", "2026-01-01T00:00:00Z", "-m", "Feature reworded")

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	_, entries, err := loadIndex(alias)
	require.NoError(t, err)
	require.NotContains(t, entries, "fixture")
	require.Len(t, entries, 1)
}
