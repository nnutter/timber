package review

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func savedFixture(t *testing.T, stack Stack, store string, reviewed bool) map[string]any {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(store, "sessions"), 0o700))
	session := map[string]any{
		"version": "1.3", "id": "fixture-session", "repo_path": stack.Repo,
		"base_commit": stack.Commits[0], "diff_source": "commit_range", "commit_range": stack.Commits,
		"files": map[string]any{
			"main.go":       map[string]any{"path": "main.go", "reviewed": reviewed, "file_comments": []any{}, "line_comments": map[string]any{}},
			"CommitMessage": map[string]any{"path": "CommitMessage", "reviewed": reviewed, "file_comments": []any{}, "line_comments": map[string]any{}},
		},
	}
	index := map[string]any{"version": "2.0", "entries": map[string]any{"fixture": []any{map[string]any{
		"kind": map[string]any{"type": "local"}, "canonical_repo_path": stack.Repo, "path": "sessions/0123456789abcdef.json",
		"display": map[string]any{"reviewed_count": 999, "comment_count": 0},
	}}}}
	writeJSON(t, filepath.Join(store, "index.json"), index)
	writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
	return session
}

func TestSavedReviewOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
		status string
	}{
		{"complete", func(map[string]any) {}, "approved"},
		{"incomplete commit message", func(s map[string]any) {
			s["files"].(map[string]any)["CommitMessage"].(map[string]any)["reviewed"] = false
		}, "incomplete"},
		{"zero files", func(s map[string]any) { s["files"] = map[string]any{} }, "incomplete"},
		{"session notes", func(s map[string]any) { s["session_notes"] = "Please revise" }, "feedback"},
		{"all comment scopes", func(s map[string]any) {
			comment := func(id string) map[string]any {
				return map[string]any{"id": id, "content": "Revise this", "comment_type": "none", "line_range": map[string]any{"start": 1, "end": 2}}
			}
			s["review_comments"] = []any{comment("review")}
			file := s["files"].(map[string]any)["main.go"].(map[string]any)
			file["reviewed"] = false
			file["file_comments"] = []any{comment("file")}
			submitted := comment("line")
			submitted["lifecycle_state"] = "submitted"
			file["line_comments"] = map[string]any{"12": []any{submitted}}
		}, "feedback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRepo(t)
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
			view, err := r.Snapshot(t.Context())
			require.NoError(t, err)
			store := t.TempDir()
			session := savedFixture(t, view.Stacks[0], store, true)
			tc.mutate(session)
			path := filepath.Join(store, "sessions/0123456789abcdef.json")
			writeJSON(t, path, session)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			saved, err := (SavedReader{Stores: []string{store}}).Read(view.Stacks[0], "")
			require.NoError(t, err)
			require.NotNil(t, saved)
			require.Equal(t, tc.status, saved.Status())
			if tc.name == "all comment scopes" {
				require.Len(t, saved.Comments, 3)
				require.Equal(t, "local_draft", saved.Comments[0]["lifecycle_state"])
				require.Equal(t, "submitted", saved.Comments[2]["lifecycle_state"])
				require.Equal(t, "12", saved.Comments[2]["stored_line"])
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSavedReviewRejectsCorruption(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]any)
		message string
	}{
		{"version", func(s map[string]any) { s["version"] = "9" }, "unsupported"},
		{"null progress", func(s map[string]any) { s["files"].(map[string]any)["main.go"].(map[string]any)["reviewed"] = nil }, "null reviewed"},
		{"null comments", func(s map[string]any) { s["review_comments"] = nil }, "null comments"},
		{"invalid notes", func(s map[string]any) { s["session_notes"] = []any{} }, "session_notes"},
		{"repo mismatch", func(s map[string]any) { s["repo_path"] = t.TempDir() }, "repository mismatch"},
		{"duplicate comment", func(s map[string]any) {
			s["review_comments"] = []any{map[string]any{"id": "same", "content": "One", "comment_type": "none"}, map[string]any{"id": "same", "content": "Two", "comment_type": "none"}}
		}, "conflicting duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRepo(t)
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
			view, err := r.Snapshot(t.Context())
			require.NoError(t, err)
			store := t.TempDir()
			session := savedFixture(t, view.Stacks[0], store, true)
			tc.mutate(session)
			writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
			_, err = (SavedReader{Stores: []string{store}}).Read(view.Stacks[0], "")
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestSavedReviewScopeAndDiscovery(t *testing.T) {
	r := testRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	store := t.TempDir()
	reader := SavedReader{Stores: []string{store}}
	saved, err := reader.Read(view.Stacks[0], "")
	require.NoError(t, err)
	require.Nil(t, saved)
	session := savedFixture(t, view.Stacks[0], store, true)
	session["commit_range"] = []string{strings.Repeat("a", 40)}
	writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
	saved, err = reader.Read(view.Stacks[0], "")
	require.NoError(t, err)
	require.Nil(t, saved)
	require.NoError(t, os.Remove(filepath.Join(store, "sessions/0123456789abcdef.json")))
	_, err = reader.Read(view.Stacks[0], "")
	require.Error(t, err)

	// Several sessions can describe one range. The one that the index recorded
	// last is the session that a review continues.
	session = savedFixture(t, view.Stacks[0], store, true)
	session["id"] = "second-session"
	writeJSON(t, filepath.Join(store, "sessions/fedcba9876543210.json"), session)
	writeJSON(t, filepath.Join(store, "index.json"), map[string]any{"version": "2.0", "entries": map[string]any{"fixture": []any{
		map[string]any{"kind": map[string]any{"type": "local"}, "canonical_repo_path": r.Dir, "path": "sessions/0123456789abcdef.json", "updated_at": "2026-01-01T00:00:00Z"},
		map[string]any{"kind": map[string]any{"type": "local"}, "canonical_repo_path": r.Dir, "path": "sessions/fedcba9876543210.json", "updated_at": "2026-02-02T00:00:00Z"},
	}}})
	saved, err = reader.Read(view.Stacks[0], "")
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "second-session", saved.ID, "the last recorded session continues the review")
	saved, err = reader.Read(view.Stacks[0], "fixture-session")
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "fixture-session", saved.ID, "a selector names an exact session")
}

func savedSession(t *testing.T, store string, stack Stack, name string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(store, "sessions"), 0o700))
	relative := "sessions/" + name + ".json"
	writeJSON(t, filepath.Join(store, relative), map[string]any{
		"version": "1.3", "id": name, "repo_path": stack.Repo,
		"base_commit": stack.Commits[0], "diff_source": "commit_range", "commit_range": stack.Commits,
		"files": map[string]any{
			"main.go":       map[string]any{"path": "main.go", "reviewed": true, "file_comments": []any{}, "line_comments": map[string]any{}},
			"CommitMessage": map[string]any{"path": "CommitMessage", "reviewed": true, "file_comments": []any{}, "line_comments": map[string]any{}},
		},
	})
	return relative
}

type savedEntry struct{ slug, canonical, relative string }

func writeSavedIndex(t *testing.T, store string, entries ...savedEntry) {
	t.Helper()
	index := map[string]any{}
	for _, entry := range entries {
		index[entry.slug] = []any{map[string]any{
			"kind":                map[string]any{"type": "local"},
			"canonical_repo_path": entry.canonical,
			"path":                entry.relative,
			"display":             map[string]any{"reviewed_count": 1, "comment_count": 0},
		}}
	}
	writeJSON(t, filepath.Join(store, "index.json"), map[string]any{"version": "2.0", "entries": index})
}

func TestSavedWriterDeletesCapturedSession(t *testing.T) {
	r := testRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
	first, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	checkout, err := WorktreePath(first.Stacks[0].Repo)
	require.NoError(t, err)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "fixup! Feature")
	current, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	store := t.TempDir()
	writer := SavedWriter{Stores: []string{store}}
	superseded := Stack{Repo: checkout, Commits: first.Stacks[0].Commits}
	captured := Stack{Repo: checkout, Commits: current.Stacks[0].Commits}
	elsewhere := Stack{Repo: t.TempDir(), Commits: current.Stacks[0].Commits}
	// A review names the feature checkout, not the checkout it saved under.
	target := Stack{Repo: r.Dir, Commits: first.Stacks[0].Commits}
	old := savedSession(t, store, superseded, "aaaaaaaaaaaaaaaa")
	kept := savedSession(t, store, captured, "bbbbbbbbbbbbbbbb")
	other := savedSession(t, store, elsewhere, "cccccccccccccccc")
	writeSavedIndex(t, store,
		savedEntry{"nnutter/timber@~aaaa/commits/first", checkout, old},
		savedEntry{"nnutter/timber@~bbbb/commits/current", checkout, kept},
		savedEntry{"nnutter/other@~cccc/commits/other", elsewhere.Repo, other},
	)
	// Only the session that captured the earlier commits is removed.
	deleted, err := writer.Delete(target)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)
	require.NoFileExists(t, filepath.Join(store, old))
	require.FileExists(t, filepath.Join(store, kept))
	require.FileExists(t, filepath.Join(store, other))
	_, entries, err := loadIndex(store)
	require.NoError(t, err)
	require.NotContains(t, entries, "nnutter/timber@~aaaa/commits/first")
	require.Contains(t, entries, "nnutter/timber@~bbbb/commits/current")
	require.Contains(t, entries, "nnutter/other@~cccc/commits/other")
	deleted, err = writer.Delete(target)
	require.NoError(t, err)
	require.Zero(t, deleted)
}

func TestSavedWriterToleratesMissingStore(t *testing.T) {
	r := testRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	deleted, err := (SavedWriter{Stores: []string{t.TempDir()}}).Delete(view.Stacks[0])
	require.NoError(t, err)
	require.Zero(t, deleted)
}
