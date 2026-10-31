package review

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/nnutter/timber/internal/timber/review/autoreview"
)

// AutoOptions bounds the stack selection and the entire evaluation run.
type AutoOptions struct {
	Start   string
	End     string
	DryRun  bool
	Timeout time.Duration
	// Progress reports each stack before its provider request. Stacks that
	// need no provider call are not reported.
	Progress func(AutoProgress)
}

// AutoProgress announces the stack that is about to be sent to the provider.
type AutoProgress struct {
	Index int
	Total int
	Stack Stack
}

// AutoEntry distinguishes model recommendations from recorded approvals.
type AutoEntry struct {
	Stack      Stack               `json:"stack"`
	Reason     string              `json:"reason"`
	Evaluation *autoreview.Verdict `json:"evaluation,omitzero"`
	Approved   bool                `json:"approved"`
}

// AutoResult is the versioned result of delegated refactor assessment.
type AutoResult struct {
	Version       int         `json:"version"`
	Command       string      `json:"command"`
	Status        string      `json:"status"`
	DryRun        bool        `json:"dryRun"`
	PolicyVersion string      `json:"policyVersion"`
	View          Snapshot    `json:"view"`
	Entries       []AutoEntry `json:"entries"`
}

// Auto evaluates captured stacks. It never lets the evaluator mutate review state.
func (s Service) Auto(ctx context.Context, evaluator autoreview.Evaluator, options AutoOptions) (result AutoResult, err error) {
	if evaluator == nil {
		return result, fmt.Errorf("automatic review requires a model evaluator")
	}
	if options.Timeout < 0 {
		return result, fmt.Errorf("automatic review timeout must not be negative")
	}
	if options.Timeout == 0 {
		options.Timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	path, err := WorktreePath(s.Repository.Dir)
	if err != nil {
		return result, err
	}
	// A dry run reads committed content and review state only, so it must not
	// hold the lock that an interactive review needs.
	var lock *os.File
	if !options.DryRun {
		lock, err = lockWorktree(path)
		if err != nil {
			return result, err
		}
		defer func() { err = errors.Join(err, lock.Close()) }()
	}
	view, state, err := s.snapshotState(ctx)
	if err != nil {
		return result, err
	}
	if lock != nil {
		if err := lock.Truncate(0); err != nil {
			return result, err
		}
		if _, err := lock.WriteString("refs/heads/" + view.Branch); err != nil {
			return result, err
		}
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
	recorded := 0
	for i, stack := range selected {
		var report func()
		if options.Progress != nil {
			report = func() {
				options.Progress(AutoProgress{
					Index: i + 1,
					Total: len(selected),
					Stack: stack,
				})
			}
		}
		entry, err := s.evaluateAuto(ctx, evaluator, stack, state, report)
		if err != nil {
			return result, resumeError(err, recorded)
		}
		if !options.DryRun && entry.Evaluation != nil {
			// Record each result as it completes so a later failure keeps the
			// provider work already paid for.
			if err := s.recordAutomatic(ctx, view, &entry); err != nil {
				return result, resumeError(err, recorded)
			}
			recorded++
		}
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

// resumeError tells the user that completed results are durable, because a
// failed run records no report of its own.
func resumeError(err error, recorded int) error {
	if recorded == 0 {
		return err
	}
	return fmt.Errorf("%w (%d completed results recorded; run again to resume)", err, recorded)
}

func (s Service) evaluateAuto(ctx context.Context, evaluator autoreview.Evaluator, stack Stack, state State, report func()) (AutoEntry, error) {
	entry := AutoEntry{Stack: stack}
	if previous := state.latest(stack); previous != nil {
		entry.Reason = alreadyReviewed(previous)
		return entry, nil
	}
	messages := make([]string, 0, len(stack.Commits))
	for _, sha := range stack.Commits {
		message, err := s.Repository.autoGit(ctx, 16*1024, "show", "--no-patch", "--format=%B", sha, "--")
		if err != nil {
			return entry, err
		}
		messages = append(messages, message)
	}
	subject, _, _ := strings.Cut(messages[0], "\n")
	if !strings.HasPrefix(subject, "Refactor:") {
		entry.Reason = "Original subject does not start with the literal Refactor: prefix"
		return entry, nil
	}
	// Check names before producing a diff so obvious credentials are not sent.
	names, err := s.Repository.autoGit(ctx, autoContextLimit, "diff", "--name-only", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-relative", stack.Base, stack.Tip, "--")
	if err != nil {
		return entry, err
	}
	for name := range strings.SplitSeq(strings.TrimSuffix(names, "\n"), "\n") {
		if strings.HasPrefix(name, "\"") || sensitiveAutoPath(name) {
			entry.Reason = "Changed paths require human review"
			return entry, nil
		}
	}
	diff, err := s.Repository.autoGit(ctx, autoreview.MaxInputBytes, slices.Concat(diffOptions, []string{stack.Base, stack.Tip, "--"})...)
	if err != nil {
		return entry, err
	}
	if strings.Contains(diff, "GIT binary patch") || strings.Contains(diff, "Subproject commit ") {
		entry.Reason = "Binary or submodule changes require human review"
		return entry, nil
	}
	input := autoreview.Input{
		Subject:  subject,
		Messages: messages,
		Diff:     diff,
	}
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
	verdict, err := evaluator.Evaluate(ctx, input, commitContext{
		repository: s.Repository,
		base:       stack.Base,
		tip:        stack.Tip,
	})
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

func alreadyReviewed(attempt *Attempt) string {
	if attempt.Automatic != nil {
		return "Model result recorded: " + attempt.Automatic.Reason
	}
	return "Current stack already has a review result: " + attempt.Status
}

// recordAutomatic appends the model result for one evaluated stack. An approve
// verdict records an approval; any other verdict records an auto_reviewed result
// that later runs report instead of asking again.
func (s Service) recordAutomatic(ctx context.Context, view Snapshot, entry *AutoEntry) error {
	verdict := entry.Evaluation
	status := "auto_reviewed"
	if verdict.Decision == "approve" {
		status = "approved"
	}
	attempt := newAttempt(entry.Stack, status)
	attempt.ApprovalSource = "model"
	attempt.Automatic = &AutomaticReview{
		PolicyVersion: autoreview.PolicyVersion,
		Decision:      verdict.Decision,
		Model:         verdict.Model,
		ResponseID:    verdict.ResponseID,
		Reason:        verdict.Reason,
		Evidence:      verdict.Evidence,
	}
	err := (Store{Path: view.Path}).Update(func(state *State) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if status == "approved" {
			// Only an approval needs the state and feedback checks.
			if err := approvalAllowed([]Stack{entry.Stack}, *state); err != nil {
				return err
			}
		}
		if err := s.unchanged(ctx, view); err != nil {
			return err
		}
		state.Attempts = append(state.Attempts, attempt)
		return nil
	})
	if err != nil {
		return err
	}
	entry.Approved = status == "approved"
	return nil
}
