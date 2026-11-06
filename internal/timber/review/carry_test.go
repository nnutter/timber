package review

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// carryFixture writes a saved session for the current stack with one of its two
// files reviewed and one line comment. The session and the name that the
// store holds follow the layout that tuicr writes.
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
	// tuicr stores the range from the newest commit to the oldest, and it
	// names the session after the repository, the anchor, and the range.
	session["commit_range"] = newestFirst(scope.Commits)
	session["base_commit"] = scope.Tip
	writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
	renameIndexKey(t, store, "fixture", tuicrName(t, reviewer.Service.Repository, scope))
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		attempt := newAttempt(view.Stacks[0], "incomplete")
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
}

// tuicrName reports the name that tuicr gives a session of a range.
func tuicrName(t *testing.T, repository Repository, scope Stack) string {
	t.Helper()
	first, err := shortSHA(t.Context(), repository, scope.Commits[0])
	require.NoError(t, err)
	tip, err := shortSHA(t.Context(), repository, scope.Tip)
	require.NoError(t, err)
	return "nnutter/timber@~" + tip + "/commits/" + first + ".." + tip
}

// renameIndexKey names a session in the store under a new name.
func renameIndexKey(t *testing.T, store, from, to string) {
	t.Helper()
	index, entries, err := loadIndex(store)
	require.NoError(t, err)
	require.NotNil(t, index)
	rows, ok := entries[from]
	require.True(t, ok, "the store names the session")
	delete(entries, from)
	entries[to] = rows
	require.NoError(t, writeIndex(store, index, entries))
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
			require.Len(t, entries, 1)
			require.Contains(t, entries, tuicrName(t, reviewer.Service.Repository, carried.Stacks[0]),
				"the session keeps the name that tuicr gives it")
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
	require.Contains(t, indexed["entries"], tuicrName(t, reviewer.Service.Repository, view.Stacks[0]))
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

	result, err := reviewer.Review(t.Context(), ReviewOptions{})
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
	carried, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	_, entries, err := loadIndex(alias)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Contains(t, entries, tuicrName(t, reviewer.Service.Repository, carried.Stacks[0]))
}

// tuicr derives the name of a session from the range and the base commit that
// the session stores. A carried session must store them the way that tuicr
// stores them, or tuicr names the session under another name.
func TestCarrySavedReviewStoresTheRangeTheWayTuicrStoresIt(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	gitTest(t, reviewer.Service.Repository.Dir, "commit", "--amend", "--allow-empty", "--date", "2026-01-01T00:00:00Z", "-m", "Feature reworded")
	carried, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	scope := carried.Stacks[0]
	checkout, err := WorktreePath(scope.Repo)
	require.NoError(t, err)
	scope.Repo = checkout
	saved, err := (SavedReader{Stores: []string{store}}).Read(scope, "")
	require.NoError(t, err)
	require.NotNil(t, saved)

	document, err := readObject(saved.Path)
	require.NoError(t, err)
	commits, err := decodeRequired[[]string](document, "commit_range")
	require.NoError(t, err)
	require.Equal(t, newestFirst(scope.Commits), commits, "tuicr stores the range from the newest commit")
	base, err := decodeRequired[string](document, "base_commit")
	require.NoError(t, err)
	require.Equal(t, scope.Tip, base, "the base commit is the tip of the range")
}

// A range of more than one commit carries, because tuicr stores the range
// from the newest commit to the oldest.
func TestCarrySavedReviewCarriesARangeOfSeveralCommits(t *testing.T) {
	reviewer, _, _ := testReviewer(t)
	dir := reviewer.Service.Repository.Dir
	gitTest(t, dir, "reset", "--hard", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "first.go"), []byte("package main\n"), 0o600))
	gitTest(t, dir, "add", ".")
	gitTest(t, dir, "commit", "-m", "Add feature\n\nChange-Id: I"+strings.Repeat("a", 40))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.go"), []byte("package main\n"), 0o600))
	gitTest(t, dir, "add", ".")
	gitTest(t, dir, "commit", "-m", "fixup! Add feature")
	view, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, view.Stacks, 1)
	require.Len(t, view.Stacks[0].Commits, 2)

	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	gitTest(t, dir, "commit", "--amend", "--no-edit", "--date", "2026-01-01T00:00:00Z")
	carried, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	scope := carried.Stacks[0]
	checkout, err := WorktreePath(scope.Repo)
	require.NoError(t, err)
	scope.Repo = checkout
	saved, err := (SavedReader{Stores: []string{store}}).Read(scope, "")
	require.NoError(t, err)
	require.NotNil(t, saved, "the review moved onto the rewritten commits")
	require.Equal(t, 2, saved.Reviewed)

	_, entries, err := loadIndex(store)
	require.NoError(t, err)
	require.Contains(t, entries, tuicrName(t, reviewer.Service.Repository, carried.Stacks[0]))
}

// A squashed commit rewrites the range of a review. The carried session must
// keep the name that tuicr gives a session, because tuicr finds a session by
// that name and writes a second session for a range that it cannot find.
func TestReviewContinuesAfterASquashedCommit(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	gitTest(t, reviewer.Service.Repository.Dir, "commit", "--amend", "--allow-empty", "--date", "2026-01-01T00:00:00Z", "-m", "Feature reworded")
	carried, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)

	carrySavedReviews(t.Context(), reviewer.Service.Repository, reviewer.Reader)

	_, entries, err := loadIndex(store)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the store names one session for the range")
	require.Contains(t, entries, tuicrName(t, reviewer.Service.Repository, carried.Stacks[0]),
		"tuicr finds the carried session under its own name")
}

// A store can hold two sessions for one range: a carried session and the
// session that tuicr wrote. The review continues with the session that the
// index recorded last, because that is the session that tuicr reopens.
func TestReviewContinuesWithTheLastRecordedSession(t *testing.T) {
	reviewer, view := carryRepository(t)
	store := t.TempDir()
	reviewer.Reader = SavedReader{Stores: []string{store}}
	carryFixture(t, reviewer, view, store)
	addSecondSession(t, store, view, "2026-02-02T00:00:00Z")
	fakeTuicr(t, "exit 0")

	result, err := reviewer.Review(t.Context(), ReviewOptions{})
	require.NoError(t, err, "the review does not stop on a range with two sessions")
	require.Equal(t, "range_complete", result.Status)
	state, err := (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	recorded := state.Attempts[len(state.Attempts)-1]
	var saved SavedReview
	require.NoError(t, json.Unmarshal(recorded.Review, &saved))
	require.Equal(t, "second-session", saved.ID, "the last recorded session continues the review")
}

// addSecondSession writes a session for the same range under a later time.
func addSecondSession(t *testing.T, store string, view Snapshot, updatedAt string) {
	t.Helper()
	scope := view.Stacks[0]
	checkout, err := WorktreePath(scope.Repo)
	require.NoError(t, err)
	session := map[string]any{
		"version": "1.3", "id": "second-session", "repo_path": checkout,
		"base_commit": scope.Tip, "diff_source": "commit_range", "commit_range": newestFirst(scope.Commits),
		"files": map[string]any{
			"main.go": map[string]any{"path": "main.go", "reviewed": true, "file_comments": []any{}, "line_comments": map[string]any{}},
		},
	}
	writeJSON(t, filepath.Join(store, "sessions/fedcba9876543210.json"), session)
	index, entries, err := loadIndex(store)
	require.NoError(t, err)
	require.NotNil(t, index)
	entry := object{
		"kind":                jsontext.Value(`{"type": "local"}`),
		"canonical_repo_path": jsontext.Value(`"` + checkout + `"`),
		"path":                jsontext.Value(`"sessions/fedcba9876543210.json"`),
		"updated_at":          jsontext.Value(`"` + updatedAt + `"`),
		"display":             jsontext.Value(`{"reviewed_count": 1, "comment_count": 0}`),
	}
	for slug, rows := range entries {
		entries[slug] = append(rows, entry)
	}
	require.NoError(t, writeIndex(store, index, entries))
}
