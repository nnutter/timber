package review

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/nnutter/timber/internal/timber/review/autoreview"
)

// Attempt retains the immutable scope and result of a review.
type Attempt struct {
	ID             string                    `json:"id"`
	Stack          Stack                     `json:"stack"`
	Status         string                    `json:"status"`
	StartedAt      string                    `json:"startedAt"`
	ApprovalSource string                    `json:"approvalSource,omitempty"`
	Automatic      *AutomaticReview          `json:"automatic,omitzero"`
	Review         jsontext.Value            `json:"review,omitempty"`
	Error          string                    `json:"error,omitempty"`
	Deferred       bool                      `json:"deferred,omitzero"`
	Response       string                    `json:"response,omitempty"`
	Replacement    *Stack                    `json:"replacement,omitzero"`
	Extra          map[string]jsontext.Value `json:",inline"`
}

// matches transfers only completed outcomes. Saved sessions and interrupted
// attempts retain their original commit scope for recovery.
func (a Attempt) matches(stack Stack) bool {
	if a.ApprovalSource == "model" {
		// A result stands only under the policy that produced it: a policy
		// change asks different questions, so earlier answers cannot count.
		if a.Automatic == nil || !isSame(a.Automatic.PolicyVersion, autoreview.PolicyVersion) {
			return false
		}
		// A model approval covers the commit messages and any context the model
		// read, so unlike a user approval it must not transfer to a rewritten
		// commit that happens to keep the same patch.
		if a.Status == "approved" {
			return sameStack(a.Stack, stack) && isSame(a.Stack.Subject, stack.Subject)
		}
	}
	if sameStack(a.Stack, stack) {
		return true
	}
	switch a.Status {
	case "approved", "feedback", "auto_reviewed":
		// A recorded model result only avoids asking again while the content is
		// unchanged, so it resets exactly like a user result.
		return samePatch(a.Stack, stack)
	default:
		return false
	}
}

func (a Attempt) resets(stack Stack) bool {
	if a.Status != "addressed" {
		return false
	}
	if a.Replacement == nil {
		return false
	}
	return sameStack(*a.Replacement, stack) || samePatch(*a.Replacement, stack)
}

func (a Attempt) validate() error {
	if a.ID == "" {
		return fmt.Errorf("review attempt requires an ID")
	}
	if _, err := time.Parse(time.RFC3339Nano, a.StartedAt); err != nil {
		return fmt.Errorf("invalid attempt start time: %w", err)
	}
	if !slices.Contains([]string{"reviewing", "approved", "feedback", "incomplete", "addressed", "auto_reviewed"}, a.Status) {
		return fmt.Errorf("invalid attempt status %q", a.Status)
	}
	if err := validateApprovalSource(a); err != nil {
		return err
	}
	if a.Replacement != nil {
		if err := a.Replacement.validate(); err != nil {
			return err
		}
	}
	return a.Stack.validate()
}

// AutomaticReview records the evidence and provenance of a model review.
type AutomaticReview struct {
	PolicyVersion string   `json:"policyVersion"`
	Decision      string   `json:"decision"`
	Model         string   `json:"model"`
	ResponseID    string   `json:"responseId"`
	Reason        string   `json:"reason"`
	Evidence      []string `json:"evidence"`
}

func validateApprovalSource(a Attempt) error {
	switch a.ApprovalSource {
	case "", "user-command":
		return validateUserApproval(a)
	case "model":
		return validateModelApproval(a)
	default:
		return fmt.Errorf("invalid approval source")
	}
}

func validateUserApproval(a Attempt) error {
	if a.Automatic != nil {
		return fmt.Errorf("automatic evidence requires a model review source")
	}
	if a.Status == "auto_reviewed" {
		return fmt.Errorf("recorded model review requires a model review source")
	}
	return nil
}

func validateModelApproval(a Attempt) error {
	if a.Automatic == nil {
		return fmt.Errorf("model review requires recorded evidence")
	}
	if err := a.Automatic.validate(); err != nil {
		return err
	}
	return validateModelStatus(a)
}

func validateModelStatus(a Attempt) error {
	switch a.Status {
	case "approved":
		if a.Automatic.Decision != "approve" {
			return fmt.Errorf("model approval requires an approve decision")
		}
	case "auto_reviewed":
		if a.Automatic.Decision == "approve" {
			return fmt.Errorf("recorded model review must not approve")
		}
	default:
		return fmt.Errorf("model review requires approved or auto_reviewed status")
	}
	return nil
}

func (a AutomaticReview) validate() error {
	for _, value := range []string{a.PolicyVersion, a.Decision, a.Model, a.ResponseID, a.Reason} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("model review requires policy, decision, model, response ID, and reason")
		}
	}
	switch a.Decision {
	case "needs-human-review", "not-eligible":
	case "approve":
		if len(a.Evidence) == 0 {
			return fmt.Errorf("model approval requires evidence")
		}
	default:
		return fmt.Errorf("invalid model decision %q", a.Decision)
	}
	for _, evidence := range a.Evidence {
		if strings.TrimSpace(evidence) == "" {
			return fmt.Errorf("model review evidence must not be empty")
		}
	}
	return nil
}

// State retains the version-1 attempt format.
type State struct {
	Version  int                       `json:"version"`
	Attempts []Attempt                 `json:"attempts"`
	Extra    map[string]jsontext.Value `json:",inline"`
}

func (s State) latest(stack Stack) *Attempt {
	var result *Attempt
	for i := range s.Attempts {
		attempt := &s.Attempts[i]
		if attempt.resets(stack) {
			result = nil
			continue
		}
		if attempt.Status == "addressed" {
			continue
		}
		if attempt.matches(stack) {
			result = attempt
		} else if samePatch(attempt.Stack, stack) {
			// A newer unfinished attempt must not expose an older approval.
			result = nil
		}
	}
	return result
}

func (s State) previous(stack Stack) *Attempt {
	for i := len(s.Attempts) - 1; i >= 0; i-- {
		attempt := &s.Attempts[i]
		if attempt.matches(stack) {
			continue
		}
		if sharesLineage(attempt.Stack, stack) {
			return attempt
		}
	}
	return nil
}

func (s State) validate() error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported review state version %d", s.Version)
	}
	if s.Attempts == nil {
		return fmt.Errorf("review state requires an attempts array")
	}
	ids := map[string]bool{}
	for _, a := range s.Attempts {
		if err := a.validate(); err != nil {
			return err
		}
		if ids[a.ID] {
			return fmt.Errorf("duplicate review attempt %s", a.ID)
		}
		ids[a.ID] = true
	}
	return nil
}

// Store serializes writers with an exclusive lock and atomic file replacement.
type Store struct{ Path string }

// Load treats only an absent state file as an empty review history.
func (s Store) Load() (State, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Version: 1, Attempts: []Attempt{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read review state %s: %w", s.Path, err)
	}
	if err := state.validate(); err != nil {
		return state, fmt.Errorf("read review state %s: %w", s.Path, err)
	}
	return state, nil
}

// Update locks a read-modify-write, not the lifetime of an interactive review.
func (s Store) Update(action func(*State) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	lockPath := s.Path + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("review state is locked: %s; remove the lock only after confirming no writer is running", lockPath)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close(), os.Remove(lockPath)) }()
	state, err := s.Load()
	if err != nil {
		return err
	}
	if err := action(&state); err != nil {
		return err
	}
	if err := state.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(state, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return s.replace(append(data, '\n'))
}

func (s Store) replace(data []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".review-*.tmp")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeIfPresent(file.Name())) }()
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.Path)
}

func newAttempt(stack Stack, status string) Attempt {
	return Attempt{ID: uuid.New().String(), Stack: stack, Status: status, StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func samePatch(a, b Stack) bool {
	if a.Repo != b.Repo {
		return false
	}
	if a.PatchHash == "" {
		return false
	}
	if a.PatchHash != b.PatchHash {
		return false
	}
	// An appended amend! or reword! commit is a new version even when the
	// aggregate diff is unchanged (for example an empty commit). An appended
	// fixup! commit carries no message, so patch identity alone decides it.
	if appendedAmends(a, b) || appendedAmends(b, a) {
		return false
	}
	return true
}

func appendedAmends(old, current Stack) bool {
	if !extendsStack(old, current) {
		return false
	}
	return current.AmendCount > old.AmendCount
}

func extendsStack(old, current Stack) bool {
	if len(current.Commits) <= len(old.Commits) {
		return false
	}
	return slices.Equal(old.Commits, current.Commits[:len(old.Commits)])
}

func sameStack(a, b Stack) bool {
	return [3]string{a.Repo, a.Base, a.Key} == [3]string{b.Repo, b.Base, b.Key} && slices.Equal(a.Commits, b.Commits)
}

func sharesLineage(a, b Stack) bool {
	if a.Repo != b.Repo {
		return false
	}
	if a.Commits[0] == b.Commits[0] {
		return true
	}
	if a.ChangeID == "" {
		return false
	}
	return a.ChangeID == b.ChangeID
}

var fullSHA = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
