package review

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fakeTuicr(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tuicr"), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func testReviewer(t *testing.T) (Reviewer, Snapshot, string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	r := testRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Feature")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	store := t.TempDir()
	return Reviewer{Service: Service{Repository: r}, Reader: SavedReader{Stores: []string{store}}, Input: strings.NewReader(""), Terminal: io.Discard}, view, store
}

func TestReviewCapturesScopeAndReadsResult(t *testing.T) {
	reviewer, view, store := testReviewer(t)
	gitTest(t, reviewer.Service.Repository.Dir, "commit", "--allow-empty", "-m", "fixup! Feature")
	view, err := reviewer.Service.Repository.Snapshot(t.Context())
	require.NoError(t, err)
	log := filepath.Join(t.TempDir(), "launch")
	t.Setenv("TEST_LAUNCH_LOG", log)
	fakeTuicr(t, `printf '%s\n' "$PWD" "$@" > "$TEST_LAUNCH_LOG"`)
	scope := view.Stacks[0]
	path, err := WorktreePath(scope.Repo)
	require.NoError(t, err)
	scope.Repo = path
	savedFixture(t, scope, store, true)
	result, err := reviewer.Review(t.Context(), ReviewOptions{Stack: "HEAD"})
	require.NoError(t, err)
	require.Equal(t, "approved", result.Status)
	require.Len(t, result.Entries, 1)
	require.NotNil(t, result.Entries[0].Attempt)
	launch, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Contains(t, string(launch), path+"\n--no-update-check\n--stdout\n-r\n"+view.Stacks[0].Base+".."+view.Stacks[0].Tip)
	require.Equal(t, view.Stacks[0].Tip, gitTest(t, path, "rev-parse", "HEAD"))
	require.NotEmpty(t, result.Entries[0].Attempt.Review)
	state, err := (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	require.Equal(t, "approved", state.Attempts[0].Status)
}

func TestReviewContinuesThroughEligibleStacksByDefault(t *testing.T) {
	for _, tc := range []struct {
		name       string
		feedback   bool
		explicit   bool
		wantStatus string
		statuses   []string
	}{
		{name: "all approved", wantStatus: "range_complete", statuses: []string{"approved", "approved", "approved", "approved"}},
		{name: "feedback", feedback: true, wantStatus: "feedback", statuses: []string{"feedback", "approved", "approved", "approved"}},
		{name: "explicit stack", explicit: true, wantStatus: "approved", statuses: []string{"approved"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reviewer, _, _ := testReviewer(t)
			r := reviewer.Service.Repository
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Second")
			gitTest(t, r.Dir, "commit", "--allow-empty", "--fixup=HEAD")
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Third")
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Fourth")
			view, err := r.Snapshot(t.Context())
			require.NoError(t, err)
			path, err := WorktreePath(r.Dir)
			require.NoError(t, err)
			log := filepath.Join(t.TempDir(), "launch")
			t.Setenv("TEST_LAUNCH_LOG", log)
			selected := view.Stacks
			options := ReviewOptions{}
			if tc.explicit {
				selected = view.Stacks[1:2]
				options.Stack = selected[0].Tip
			}
			wantLaunch := ""
			for i, stack := range selected {
				scope := stack
				scope.Repo = path
				store := t.TempDir()
				reviewer.Reader.Stores = append(reviewer.Reader.Stores, store)
				session := savedFixture(t, scope, store, true)
				if tc.feedback {
					if i == 0 {
						session["session_notes"] = "Please revise"
						writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)
					}
				}
				wantLaunch += stack.Base + ".." + stack.Tip + "\n"
			}
			fakeTuicr(t, `printf '%s\n' "$4" >> "$TEST_LAUNCH_LOG"`)

			result, err := reviewer.Review(t.Context(), options)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, result.Status)
			launch, err := os.ReadFile(log)
			require.NoError(t, err)
			require.Equal(t, wantLaunch, string(launch))
			state, err := (Store{Path: view.Path}).Load()
			require.NoError(t, err)
			require.Len(t, state.Attempts, len(tc.statuses))
			for i, want := range tc.statuses {
				require.Equal(t, selected[i].Commits, state.Attempts[i].Stack.Commits)
				require.Equal(t, want, state.Attempts[i].Status)
			}
		})
	}
}

// A review resumes the attempt that captured the same stack, and a stack that a
// fixup changed starts a new attempt.
func TestInterruptedReviewResumesTheCapturedAttempt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		amend      bool
		wantStatus string
		wantSame   bool
	}{
		{name: "same stack resumes", wantStatus: "range_complete", wantSame: true},
		{name: "changed stack replaces", amend: true, wantStatus: "range_complete", wantSame: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reviewer, view, store := testReviewer(t)
			fakeTuicr(t, "exit 1")
			_, err := reviewer.Review(t.Context(), ReviewOptions{})
			require.ErrorContains(t, err, "tuicr interrupted")
			path, err := WorktreePath(view.Stacks[0].Repo)
			require.NoError(t, err)
			scope := view.Stacks[0]
			scope.Repo = path
			if tc.amend {
				gitTest(t, reviewer.Service.Repository.Dir, "commit", "--allow-empty", "--fixup=HEAD")
				rewritten, err := reviewer.Service.Repository.Snapshot(t.Context())
				require.NoError(t, err)
				scope = rewritten.Stacks[0]
				scope.Repo = path
			}
			savedFixture(t, scope, store, true)
			before, err := (Store{Path: view.Path}).Load()
			require.NoError(t, err)
			require.Len(t, before.Attempts, 1)
			fakeTuicr(t, "exit 0")
			result, err := reviewer.Review(t.Context(), ReviewOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, result.Status)
			after, err := (Store{Path: view.Path}).Load()
			require.NoError(t, err)
			require.Len(t, after.Attempts, 1)
			resumed := after.Attempts[0].ID == before.Attempts[0].ID
			require.Equal(t, tc.wantSame, resumed)
		})
	}
}

// A saved session that still has work left belongs to a current stack, so a
// recovery must resume that attempt and keep the progress it recorded.
func TestInterruptedReviewResumesSavedProgress(t *testing.T) {
	reviewer, view, store := testReviewer(t)
	fakeTuicr(t, "exit 1")
	_, err := reviewer.Review(t.Context(), ReviewOptions{})
	require.ErrorContains(t, err, "tuicr interrupted")
	path, err := WorktreePath(view.Stacks[0].Repo)
	require.NoError(t, err)
	scope := view.Stacks[0]
	scope.Repo = path
	session := savedFixture(t, scope, store, false)
	session["files"].(map[string]any)["main.go"].(map[string]any)["reviewed"] = true
	writeJSON(t, filepath.Join(store, "sessions/0123456789abcdef.json"), session)

	fakeTuicr(t, "exit 0")
	before, err := (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	result, err := reviewer.Review(t.Context(), ReviewOptions{})
	require.NoError(t, err)
	require.Equal(t, "incomplete", result.Status)
	require.Equal(t, before.Attempts[0].ID, result.Entries[0].Attempt.ID)
	state, err := (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	require.Len(t, state.Attempts, 1, "recovery must not discard a resumable attempt")
	require.Equal(t, before.Attempts[0].ID, state.Attempts[0].ID)
	var saved SavedReview
	require.NoError(t, json.Unmarshal(state.Attempts[0].Review, &saved))
	require.Equal(t, "incomplete", saved.Status())
	require.Equal(t, 1, saved.Reviewed)
	require.Equal(t, 2, saved.Files)
}

func TestReviewDiscardsUntouchedExit(t *testing.T) {
	for _, savedSession := range []bool{false, true} {
		t.Run(fmt.Sprintf("saved=%t", savedSession), func(t *testing.T) {
			reviewer, view, store := testReviewer(t)
			fakeTuicr(t, "exit 0")
			if savedSession {
				scope := view.Stacks[0]
				path, err := WorktreePath(scope.Repo)
				require.NoError(t, err)
				scope.Repo = path
				savedFixture(t, scope, store, false)
			}
			for range 2 {
				result, err := reviewer.Review(t.Context(), ReviewOptions{})
				require.NoError(t, err)
				require.Equal(t, "cancelled", result.Status)
				state, err := (Store{Path: view.Path}).Load()
				require.NoError(t, err)
				require.Empty(t, state.Attempts)
			}
		})
	}
}

func TestReviewReusesDisposableCheckoutAtSelectedTip(t *testing.T) {
	reviewer, _, _ := testReviewer(t)
	r := reviewer.Service.Repository
	fakeTuicr(t, "exit 0")
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "file.txt"), []byte("first version"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, ".gitignore"), []byte("artifact.txt\n"), 0o600))
	gitTest(t, r.Dir, "add", ".")
	gitTest(t, r.Dir, "commit", "--amend", "--no-edit")
	first := gitTest(t, r.Dir, "rev-parse", "HEAD")
	result, err := reviewer.Review(t.Context(), ReviewOptions{Stack: first})
	require.NoError(t, err)
	require.Equal(t, "cancelled", result.Status)
	path, err := WorktreePath(r.Dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(path, "file.txt"), []byte("incidental edit"), 0o600))
	for _, name := range []string{"local.txt", "artifact.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(path, name), []byte("disposable"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "file.txt"), []byte("second version"), 0o600))
	gitTest(t, r.Dir, "add", "file.txt")
	gitTest(t, r.Dir, "commit", "-m", "Second version")
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "local.txt"), []byte("preserve feature checkout"), 0o600))
	result, err = reviewer.Review(t.Context(), ReviewOptions{Stack: "HEAD"})
	require.NoError(t, err)
	require.Equal(t, "cancelled", result.Status)
	content, err := os.ReadFile(filepath.Join(path, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "second version", string(content))
	require.Equal(t, "HEAD", gitTest(t, path, "rev-parse", "--abbrev-ref", "HEAD"))
	require.Empty(t, gitTest(t, path, "status", "--porcelain", "--ignored"))
	content, err = os.ReadFile(filepath.Join(r.Dir, "local.txt"))
	require.NoError(t, err)
	require.Equal(t, "preserve feature checkout", string(content))
}

func TestDeferredReviewsRemainPending(t *testing.T) {
	reviewer, view, _ := testReviewer(t)
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		attempt := newAttempt(view.Stacks[0], "incomplete")
		attempt.Deferred = true
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
	result, err := reviewer.Review(t.Context(), ReviewOptions{})
	require.NoError(t, err)
	require.Equal(t, "no_waiting", result.Status)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "incomplete", result.Entries[0].Status)
}

// A review holds the worktree lock for the whole of tuicr, so nothing else can
// start a review, approve, or rewrite the state while the human reviews.
func TestReviewHoldsActivityLockDuringLaunch(t *testing.T) {
	reviewer, _, _ := testReviewer(t)
	started := filepath.Join(t.TempDir(), "started")
	released := filepath.Join(t.TempDir(), "released")
	t.Setenv("TEST_STARTED", started)
	t.Setenv("TEST_RELEASED", released)
	fakeTuicr(t, `printf started > "$TEST_STARTED"
while [ ! -f "$TEST_RELEASED" ]; do sleep 0.05; done`)
	finished := make(chan error, 1)
	go func() {
		_, err := reviewer.Review(t.Context(), ReviewOptions{})
		finished <- err
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(started)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	running, err := reviewer.Service.ReviewRunning(t.Context())
	require.NoError(t, err)
	require.True(t, running, "the launch is part of the live review")
	require.NoError(t, os.WriteFile(released, nil, 0o600))
	require.NoError(t, <-finished)
	running, err = reviewer.Service.ReviewRunning(t.Context())
	require.NoError(t, err)
	require.False(t, running)
}

func TestRecoverablePriorIgnoresRecordedModelResults(t *testing.T) {
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	model := newAttempt(view.Stacks[0], "auto_reviewed")
	model.ApprovalSource = "model"
	model.Automatic = &AutomaticReview{PolicyVersion: "refactor-v1", Decision: "needs-human-review", Model: "test-model", ResponseID: "resp_test", Reason: "Equivalence is unclear."}
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, model)
		return nil
	}))
	// A recorded model result has no saved session, so it is not a recovery target.
	state, err := (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	require.Nil(t, recoverablePrior(state, view, view.Stacks[0]))
	// A human attempt still is, even when it follows the model result.
	human := newAttempt(view.Stacks[0], "incomplete")
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, human)
		return nil
	}))
	state, err = (Store{Path: view.Path}).Load()
	require.NoError(t, err)
	prior := recoverablePrior(state, view, view.Stacks[0])
	require.NotNil(t, prior)
	require.Equal(t, human.ID, prior.ID)
}

func TestReviewDiscardsEarlierStackVersion(t *testing.T) {
	for _, status := range []string{"incomplete", "reviewing"} {
		t.Run(status, func(t *testing.T) {
			reviewer, view, store := testReviewer(t)
			r := reviewer.Service.Repository
			first := view.Stacks[0]
			path, err := WorktreePath(first.Repo)
			require.NoError(t, err)
			// The review of the version before the fixup commit left a session.
			earlier := savedSession(t, store, Stack{Repo: path, Commits: first.Commits}, "aaaaaaaaaaaaaaaa")
			require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
				state.Attempts = append(state.Attempts, newAttempt(first, status))
				return nil
			}))
			// A fixup commit extends the stack, so its captured range is gone.
			gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "fixup! Feature")
			view, err = r.Snapshot(t.Context())
			require.NoError(t, err)
			scope := view.Stacks[0]
			scope.Repo = path
			current := savedSession(t, store, scope, "bbbbbbbbbbbbbbbb")
			writeSavedIndex(t, store,
				savedEntry{"nnutter/timber@~aaaa/commits/first", path, earlier},
				savedEntry{"nnutter/timber@~bbbb/commits/current", path, current},
			)
			log := filepath.Join(t.TempDir(), "launch")
			t.Setenv("TEST_LAUNCH_LOG", log)
			fakeTuicr(t, `printf '%s\n' "$4" > "$TEST_LAUNCH_LOG"`)
			result, err := reviewer.Review(t.Context(), ReviewOptions{})
			require.NoError(t, err)
			require.Equal(t, "range_complete", result.Status)
			launch, err := os.ReadFile(log)
			require.NoError(t, err)
			require.Equal(t, view.Stacks[0].Base+".."+view.Stacks[0].Tip, strings.TrimSpace(string(launch)))
			// The earlier review and its session are gone, the new one remains.
			state, err := (Store{Path: view.Path}).Load()
			require.NoError(t, err)
			require.Len(t, state.Attempts, 1)
			require.Equal(t, "approved", state.Attempts[0].Status)
			require.NoFileExists(t, filepath.Join(store, earlier))
			require.FileExists(t, filepath.Join(store, current))
		})
	}
}
