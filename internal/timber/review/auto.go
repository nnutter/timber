package review

import (
	"fmt"
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

// resumeError tells the user that completed results are durable, because a
// failed run records no report of its own.
func resumeError(err error, recorded int) error {
	if recorded == 0 {
		return err
	}
	return fmt.Errorf("%w (%d completed results recorded; run again to resume)", err, recorded)
}

func alreadyReviewed(attempt *Attempt) string {
	if attempt.Automatic != nil {
		return "Model result recorded: " + attempt.Automatic.Reason
	}
	return "Current stack already has a review result: " + attempt.Status
}
