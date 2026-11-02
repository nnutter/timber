package review

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/nnutter/timber/internal/timber/review/autoreview"
)

func autoCommitMessages(ctx context.Context, repository Repository, stack Stack) ([]string, error) {
	messages := make([]string, 0, len(stack.Commits))
	for _, sha := range stack.Commits {
		message, err := repository.autoGit(ctx, 16*1024, "show", "--no-patch", "--format=%B", sha, "--")
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func sensitiveAutoNames(ctx context.Context, repository Repository, stack Stack) (bool, error) {
	// Check names before producing a diff so obvious credentials are not sent.
	names, err := repository.autoGit(ctx, autoContextLimit, "diff", "--name-only", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-relative", stack.Base, stack.Tip, "--")
	if err != nil {
		return false, err
	}
	for name := range strings.SplitSeq(strings.TrimSuffix(names, "\n"), "\n") {
		if strings.HasPrefix(name, "\"") || sensitiveAutoPath(name) {
			return true, nil
		}
	}
	return false, nil
}

func validateAutoEvaluator(evaluator autoreview.Evaluator, options *AutoOptions) error {
	if evaluator == nil {
		return fmt.Errorf("automatic review requires a model evaluator")
	}
	if options.Timeout < 0 {
		return fmt.Errorf("automatic review timeout must not be negative")
	}
	if options.Timeout == 0 {
		options.Timeout = 3 * time.Minute
	}
	return nil
}

func autoLock(s Service, options AutoOptions) (*os.File, error) {
	path, err := WorktreePath(s.Repository.Dir)
	if err != nil {
		return nil, err
	}
	// A dry run reads committed content and review state only, so it must not
	// hold the lock that an interactive review needs.
	if options.DryRun {
		return nil, nil
	}
	return lockWorktree(path)
}

func runAuto(ctx context.Context, s Service, evaluator autoreview.Evaluator, options AutoOptions, lock *os.File) (AutoResult, error) {
	var result AutoResult
	view, state, err := s.snapshotState(ctx)
	if err != nil {
		return result, err
	}
	if err := writeAutoLock(lock, view); err != nil {
		return result, err
	}
	if len(activeAttempts(state)) != 0 {
		return result, fmt.Errorf("recover or replace the active review before automatic approval")
	}
	selected, err := (Reviewer{Service: s}).bounds(ctx, view, options.Start, options.End)
	if err != nil {
		return result, err
	}
	result = AutoResult{
		Version:       1,
		Command:       "auto",
		Status:        "complete",
		DryRun:        options.DryRun,
		PolicyVersion: autoreview.PolicyVersion,
		View:          view,
		Entries:       []AutoEntry{},
	}
	if options.DryRun {
		result.Status = "dry-run"
	}
	return recordAutoEntries(ctx, s, evaluator, options, view, state, selected, result)
}

func writeAutoLock(lock *os.File, view Snapshot) error {
	if lock == nil {
		return nil
	}
	if err := lock.Truncate(0); err != nil {
		return err
	}
	_, err := lock.WriteString("refs/heads/" + view.Branch)
	return err
}

func recordAutoEntries(ctx context.Context, s Service, evaluator autoreview.Evaluator, options AutoOptions, view Snapshot, state State, selected []Stack, result AutoResult) (AutoResult, error) {
	recorded := 0
	for i, stack := range selected {
		entry, next, err := recordOneAuto(ctx, s, evaluator, options, view, state, selected, stack, i, recorded)
		if err != nil {
			return result, err
		}
		recorded = next
		result.Entries = append(result.Entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return result, resumeError(err, recorded)
	}
	if err := s.unchanged(ctx, view); err != nil {
		return result, resumeError(err, recorded)
	}
	return result, nil
}

func recordOneAuto(ctx context.Context, s Service, evaluator autoreview.Evaluator, options AutoOptions, view Snapshot, state State, selected []Stack, stack Stack, index, recorded int) (AutoEntry, int, error) {
	var report func()
	if options.Progress != nil {
		report = func() {
			options.Progress(AutoProgress{Index: index + 1, Total: len(selected), Stack: stack})
		}
	}
	entry, err := s.evaluateAuto(ctx, evaluator, stack, state, report)
	if err != nil {
		return entry, recorded, resumeError(err, recorded)
	}
	if options.DryRun || isSame(entry.Evaluation, nil) {
		return entry, recorded, nil
	}
	// Record each result as it completes so a later failure keeps the
	// provider work already paid for.
	if err := s.recordAutomatic(ctx, view, &entry); err != nil {
		return entry, recorded, resumeError(err, recorded)
	}
	return entry, recorded + 1, nil
}

func finishAutoEvaluation(ctx context.Context, s Service, evaluator autoreview.Evaluator, stack Stack, entry AutoEntry, subject string, messages []string, report func()) (AutoEntry, error) {
	diff, err := s.Repository.autoGit(ctx, autoreview.MaxInputBytes, slices.Concat(diffOptions, []string{stack.Base, stack.Tip, "--"})...)
	if err != nil {
		return entry, err
	}
	if strings.Contains(diff, "GIT binary patch") || strings.Contains(diff, "Subproject commit ") {
		entry.Reason = "Binary or submodule changes require human review"
		return entry, nil
	}
	input := autoreview.Input{Subject: subject, Messages: messages, Diff: diff}
	encoded, err := json.Marshal(input)
	if err != nil {
		return entry, err
	}
	if len(encoded) > autoreview.MaxInputBytes {
		return entry, fmt.Errorf("automatic review input exceeds %d bytes", autoreview.MaxInputBytes)
	}
	if report != nil {
		report()
	}
	verdict, err := evaluator.Evaluate(ctx, input, commitContext{repository: s.Repository, base: stack.Base, tip: stack.Tip})
	if err != nil {
		return entry, fmt.Errorf("evaluate stack %s: %w", stack.Key, err)
	}
	if err := verdict.Validate(); err != nil {
		return entry, err
	}
	entry.Evaluation = &verdict
	entry.Reason = verdict.Reason
	return entry, nil
}
