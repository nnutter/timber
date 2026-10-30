package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAutomaticReviewDoesNotTransferToRewrittenCommits(t *testing.T) {
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	attempt := newAttempt(view.Stacks[0], "approved")
	attempt.ApprovalSource = "model"
	attempt.Automatic = &AutomaticReview{PolicyVersion: "refactor-v1", Decision: "approve", Model: "test-model", ResponseID: "resp_test", Reason: "Only the local constant name changes.", Evidence: []string{"value:1 preserves the value"}}
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
	service := Service{Repository: r}
	before, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "approved", before.Entries[0].Status)
	require.Equal(t, attempt.Automatic, before.Entries[0].Attempt.Automatic)
	gitTest(t, r.Dir, "commit", "--amend", "-m", "Change behavior")
	after, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, view.Stacks[0].PatchHash, after.View.Stacks[0].PatchHash)
	require.Equal(t, "needs_review", after.Entries[0].Status)
}

func TestAutomaticReviewResetsOnlyWhenContentChanges(t *testing.T) {
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	attempt := newAttempt(view.Stacks[0], "auto_reviewed")
	attempt.ApprovalSource = "model"
	attempt.Automatic = &AutomaticReview{PolicyVersion: "refactor-v1", Decision: "needs-human-review", Model: "test-model", ResponseID: "resp_test", Reason: "Equivalence is unclear."}
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
	service := Service{Repository: r}
	before, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "auto_reviewed", before.Entries[0].Status)
	gitTest(t, r.Dir, "commit", "--amend", "-m", "Refactor: Extract named constant")
	rewritten, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, view.Stacks[0].PatchHash, rewritten.View.Stacks[0].PatchHash)
	require.Equal(t, "auto_reviewed", rewritten.Entries[0].Status)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("two\n"), 0o600))
	gitTest(t, r.Dir, "commit", "-am", "fixup! Refactor: Extract named constant")
	changed, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "needs_review", changed.Entries[0].Status)
}

func TestAutomaticReviewValidation(t *testing.T) {
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	base := newAttempt(view.Stacks[0], "auto_reviewed")
	base.ApprovalSource = "model"
	base.Automatic = &AutomaticReview{PolicyVersion: "refactor-v1", Decision: "needs-human-review", Model: "test-model", ResponseID: "resp_test", Reason: "Equivalence is unclear."}
	require.NoError(t, base.validate())
	for _, tc := range []struct {
		name   string
		change func(*Attempt)
	}{
		{"missing source", func(a *Attempt) { a.ApprovalSource = "" }},
		{"missing evidence", func(a *Attempt) { a.Automatic = nil }},
		{"approving decision", func(a *Attempt) { a.Automatic.Decision = "approve" }},
		{"unknown decision", func(a *Attempt) { a.Automatic.Decision = "maybe" }},
		{"missing reason", func(a *Attempt) { a.Automatic.Reason = " " }},
		{"approval without evidence", func(a *Attempt) {
			a.Status = "approved"
			a.Automatic.Decision = "approve"
		}},
		{"approval with human decision", func(a *Attempt) {
			a.Status = "approved"
			a.Automatic.Evidence = []string{"value is unchanged"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempt := base
			automatic := *base.Automatic
			attempt.Automatic = &automatic
			tc.change(&attempt)
			require.Error(t, attempt.validate())
		})
	}
}

func TestAutomaticReviewUnderAnotherPolicyDoesNotApply(t *testing.T) {
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	attempt := newAttempt(view.Stacks[0], "approved")
	attempt.ApprovalSource = "model"
	attempt.Automatic = &AutomaticReview{PolicyVersion: "refactor-old", Decision: "approve", Model: "test-model", ResponseID: "resp_test", Reason: "Equivalent.", Evidence: []string{"value:1 is unchanged"}}
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, attempt)
		return nil
	}))
	status, err := (Service{Repository: r}).Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "needs_review", status.Entries[0].Status)
	// The result stays in the history that a reader can still inspect.
	require.NotNil(t, status.Entries[0].Previous)
}
