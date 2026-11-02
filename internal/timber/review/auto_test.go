package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber/review/autoreview"
)

type evaluatorFunc func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error)

func (f evaluatorFunc) Evaluate(ctx context.Context, input autoreview.Input, reader autoreview.ContextReader) (autoreview.Verdict, error) {
	return f(ctx, input, reader)
}

func approvingEvaluator() autoreview.Evaluator {
	return evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		return autoreview.Verdict{Decision: "approve", Reason: "The value is preserved.", Evidence: []string{"value:1 is unchanged"}, Model: "test-model", ResponseID: "resp_test"}, nil
	})
}

func autoRepo(t *testing.T) Repository {
	t.Helper()
	r := testRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("one\n"), 0o600))
	gitTest(t, r.Dir, "add", "value")
	gitTest(t, r.Dir, "commit", "-m", "Refactor: Extract constant")
	return r
}

func TestAutoDryRunEvaluatesCompleteStackWithoutRecordingApproval(t *testing.T) {
	r := autoRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("two\n"), 0o600))
	gitTest(t, r.Dir, "commit", "-am", "fixup! Refactor: Extract constant")
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("uncommitted\n"), 0o600))
	var calls int
	evaluator := evaluatorFunc(func(ctx context.Context, input autoreview.Input, reader autoreview.ContextReader) (autoreview.Verdict, error) {
		calls++
		require.Contains(t, input.Diff, "+two")
		require.NotContains(t, input.Diff, "uncommitted")
		require.Len(t, input.Messages, 2)
		text, err := reader.ReadFile(ctx, "tip", "value")
		require.NoError(t, err)
		require.Equal(t, "two\n", text)
		return approvingEvaluator().Evaluate(ctx, input, reader)
	})
	service := Service{Repository: r}
	result, err := service.Auto(t.Context(), evaluator, AutoOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "auto", result.Command)
	require.True(t, result.DryRun)
	require.Len(t, result.Entries, 1)
	require.Len(t, result.Entries[0].Stack.Commits, 2)
	require.False(t, result.Entries[0].Approved)
	require.Equal(t, "approve", result.Entries[0].Evaluation.Decision)
	_, err = os.Stat(result.View.Path)
	require.ErrorIs(t, err, os.ErrNotExist)
	status, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "needs_review", status.Entries[0].Status)
}

func TestAutoRecordsModelEvidenceAndSkipsApprovedStack(t *testing.T) {
	r := autoRepo(t)
	service := Service{Repository: r}
	result, err := service.Auto(t.Context(), approvingEvaluator(), AutoOptions{})
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	require.True(t, result.Entries[0].Approved)
	status, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "approved", status.Entries[0].Status)
	attempt := status.Entries[0].Attempt
	require.Equal(t, "model", attempt.ApprovalSource)
	require.Equal(t, autoreview.PolicyVersion, attempt.Automatic.PolicyVersion)
	require.Equal(t, "test-model", attempt.Automatic.Model)
	require.Equal(t, "resp_test", attempt.Automatic.ResponseID)
	_, err = service.Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		require.Fail(t, "approved stacks must not be evaluated again")
		return autoreview.Verdict{}, nil
	}), AutoOptions{})
	require.NoError(t, err)
}

func TestAutoFailsClosedDuringEvaluation(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T, Repository, context.Context) (autoreview.Verdict, error)
		want string
	}{
		{"provider error", func(*testing.T, Repository, context.Context) (autoreview.Verdict, error) {
			return autoreview.Verdict{}, fmt.Errorf("provider unavailable")
		}, "provider unavailable"},
		{"invalid verdict", func(*testing.T, Repository, context.Context) (autoreview.Verdict, error) {
			return autoreview.Verdict{Decision: "approve", Model: "test-model", ResponseID: "resp_invalid"}, nil
		}, "reason"},
		{"history changed", func(t *testing.T, r Repository, ctx context.Context) (autoreview.Verdict, error) {
			gitTest(t, r.Dir, "commit", "--amend", "-m", "Refactor: Different subject")
			return approvingEvaluator().Evaluate(ctx, autoreview.Input{}, nil)
		}, "history changed"},
		{"branch changed", func(t *testing.T, r Repository, ctx context.Context) (autoreview.Verdict, error) {
			gitTest(t, r.Dir, "checkout", "-b", "other")
			gitTest(t, r.Dir, "branch", "--set-upstream-to=main")
			return approvingEvaluator().Evaluate(ctx, autoreview.Input{}, nil)
		}, "branch changed"},
		{"concurrent feedback", func(t *testing.T, r Repository, ctx context.Context) (autoreview.Verdict, error) {
			view, err := r.Snapshot(ctx)
			require.NoError(t, err)
			require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
				state.Attempts = append(state.Attempts, newAttempt(view.Stacks[0], "feedback"))
				return nil
			}))
			return approvingEvaluator().Evaluate(ctx, autoreview.Input{}, nil)
		}, "pending feedback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := autoRepo(t)
			view, err := r.Snapshot(t.Context())
			require.NoError(t, err)
			_, err = (Service{Repository: r}).Auto(t.Context(), evaluatorFunc(func(ctx context.Context, _ autoreview.Input, _ autoreview.ContextReader) (autoreview.Verdict, error) {
				return tc.run(t, r, ctx)
			}), AutoOptions{})
			require.ErrorContains(t, err, tc.want)
			state, err := (Store{Path: view.Path}).Load()
			require.NoError(t, err)
			for _, attempt := range state.Attempts {
				require.NotEqual(t, "model", attempt.ApprovalSource)
			}
		})
	}
}

func TestAutoResumesAfterALaterStackFails(t *testing.T) {
	r := autoRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Refactor: Second change")
	service := Service{Repository: r}
	calls := 0
	_, err := service.Auto(t.Context(), evaluatorFunc(func(ctx context.Context, input autoreview.Input, reader autoreview.ContextReader) (autoreview.Verdict, error) {
		calls++
		if calls == 2 {
			return autoreview.Verdict{}, fmt.Errorf("second request failed")
		}
		return approvingEvaluator().Evaluate(ctx, input, reader)
	}), AutoOptions{})
	require.ErrorContains(t, err, "second request failed")
	require.ErrorContains(t, err, "1 completed results recorded")
	require.Equal(t, 2, calls)
	// The completed stack keeps its approval, and the failed one stays pending.
	status, err := service.Status(t.Context(), false)
	require.NoError(t, err)
	require.Equal(t, "approved", status.Entries[0].Status)
	require.Equal(t, "needs_review", status.Entries[1].Status)
	// A later run resumes with the stacks that have no result yet.
	calls = 0
	result, err := service.Auto(t.Context(), evaluatorFunc(func(ctx context.Context, input autoreview.Input, reader autoreview.ContextReader) (autoreview.Verdict, error) {
		calls++
		return approvingEvaluator().Evaluate(ctx, input, reader)
	}), AutoOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, result.Entries, 2)
	require.True(t, result.Entries[1].Approved)
}

func TestAutoSkipsIneligibleAndPreviouslyReviewedStacks(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		status  string
	}{
		{"ordinary subject", "Add behavior", ""},
		{"wrong case", "refactor: Rename constant", ""},
		{"leading space", " Refactor: Rename constant", ""},
		{"pending feedback", "Refactor: Rename constant", "feedback"},
		{"incomplete review", "Refactor: Rename constant", "incomplete"},
		{"explicit approval", "Refactor: Rename constant", "approved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := autoRepo(t)
			gitTest(t, r.Dir, "commit", "--amend", "--cleanup=verbatim", "-m", tc.subject)
			view, err := r.Snapshot(t.Context())
			require.NoError(t, err)
			if tc.status != "" {
				require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
					state.Attempts = append(state.Attempts, newAttempt(view.Stacks[0], tc.status))
					return nil
				}))
			}
			result, err := (Service{Repository: r}).Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
				require.Fail(t, "ineligible stacks must not call the provider")
				return autoreview.Verdict{}, nil
			}), AutoOptions{})
			require.NoError(t, err)
			require.Len(t, result.Entries, 1)
			require.Nil(t, result.Entries[0].Evaluation)
			require.NotEmpty(t, result.Entries[0].Reason)
			require.False(t, result.Entries[0].Approved)
		})
	}
}

func TestAutoAbstentionRecordsResultWithoutApproving(t *testing.T) {
	for _, decision := range []string{"needs-human-review", "not-eligible"} {
		t.Run(decision, func(t *testing.T) {
			r := autoRepo(t)
			service := Service{Repository: r}
			result, err := service.Auto(t.Context(), abstainingEvaluator(decision), AutoOptions{})
			require.NoError(t, err)
			require.False(t, result.Entries[0].Approved)
			status, err := service.Status(t.Context(), false)
			require.NoError(t, err)
			require.Equal(t, "auto_reviewed", status.Entries[0].Status)
			require.Equal(t, "model", status.Entries[0].Attempt.ApprovalSource)
			require.Equal(t, decision, status.Entries[0].Attempt.Automatic.Decision)
			require.Equal(t, "Cannot establish equivalence.", status.Entries[0].Attempt.Automatic.Reason)
			counts := map[string]int{}
			for _, count := range status.Summary {
				counts[count.State] = count.Count
			}
			require.Equal(t, 1, counts["auto_reviewed"])
			require.Equal(t, 0, counts["needs_review"])
		})
	}
}

func abstainingEvaluator(decision string) autoreview.Evaluator {
	return evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		return autoreview.Verdict{Decision: decision, Reason: "Cannot establish equivalence.", Evidence: []string{}, Model: "test-model", ResponseID: "resp_abstain"}, nil
	})
}

func TestAutoSkipsRecordedResultUntilContentChanges(t *testing.T) {
	r := autoRepo(t)
	service := Service{Repository: r}
	_, err := service.Auto(t.Context(), abstainingEvaluator("needs-human-review"), AutoOptions{})
	require.NoError(t, err)
	// The recorded result answers a second run without another request.
	result, err := service.Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		require.Fail(t, "recorded results must not be evaluated again")
		return autoreview.Verdict{}, nil
	}), AutoOptions{})
	require.NoError(t, err)
	require.Contains(t, result.Entries[0].Reason, "Cannot establish equivalence.")
	// Changed content resets the recorded result.
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte("three\n"), 0o600))
	gitTest(t, r.Dir, "commit", "-am", "fixup! Refactor: Extract constant")
	calls := 0
	_, err = service.Auto(t.Context(), evaluatorFunc(func(ctx context.Context, input autoreview.Input, reader autoreview.ContextReader) (autoreview.Verdict, error) {
		calls++
		return abstainingEvaluator("needs-human-review").Evaluate(ctx, input, reader)
	}), AutoOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestAutoRejectsOversizedDiffWithoutCallingProvider(t *testing.T) {
	r := autoRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "value"), []byte(strings.Repeat("a", autoreview.MaxInputBytes+1)), 0o600))
	gitTest(t, r.Dir, "commit", "-am", "fixup! Refactor: Extract constant")
	_, err := (Service{Repository: r}).Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		require.Fail(t, "oversized input must not call the provider")
		return autoreview.Verdict{}, nil
	}), AutoOptions{})
	require.Error(t, err)
}

func TestAutoReportsOnlyProviderRequests(t *testing.T) {
	r := autoRepo(t)
	gitTest(t, r.Dir, "commit", "--allow-empty", "-m", "Add behavior")
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	var reported []AutoProgress
	result, err := (Service{Repository: r}).Auto(t.Context(), approvingEvaluator(), AutoOptions{
		Progress: func(progress AutoProgress) { reported = append(reported, progress) },
	})
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	require.Len(t, reported, 1)
	require.Equal(t, 1, reported[0].Index)
	require.Equal(t, 2, reported[0].Total)
	require.Equal(t, view.Stacks[0].Key, reported[0].Stack.Key)
}

func TestAutoSkipsBinaryChangesBeforeAnyRequest(t *testing.T) {
	r := autoRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "blob.bin"), []byte{0, 1, 2}, 0o600))
	gitTest(t, r.Dir, "add", "blob.bin")
	gitTest(t, r.Dir, "commit", "-m", "fixup! Refactor: Extract constant")
	result, err := (Service{Repository: r}).Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		require.Fail(t, "binary changes must not reach the provider")
		return autoreview.Verdict{}, nil
	}), AutoOptions{})
	require.NoError(t, err)
	require.Contains(t, result.Entries[0].Reason, "Binary or submodule changes require human review")
}

func TestAutoRequiresNoActiveReview(t *testing.T) {
	r := autoRepo(t)
	view, err := r.Snapshot(t.Context())
	require.NoError(t, err)
	require.NoError(t, (Store{Path: view.Path}).Update(func(state *State) error {
		state.Attempts = append(state.Attempts, newAttempt(view.Stacks[0], "reviewing"))
		return nil
	}))
	_, err = (Service{Repository: r}).Auto(t.Context(), evaluatorFunc(func(context.Context, autoreview.Input, autoreview.ContextReader) (autoreview.Verdict, error) {
		require.Fail(t, "an active review must block automatic approval")
		return autoreview.Verdict{}, nil
	}), AutoOptions{})
	require.ErrorContains(t, err, "recover or replace the active review")
}
