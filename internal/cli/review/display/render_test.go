package display

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber/review"
)

func TestEntryStatusLabelNamesModelResults(t *testing.T) {
	stack := review.Stack{Repo: "/tmp/repo", Commits: []string{"aaaa1111"}, Tip: "aaaa1111", Subject: "Refactor: Example"}
	for _, tc := range []struct {
		name    string
		attempt *review.Attempt
		want    string
	}{
		{"unreviewed", nil, "Needs review"},
		{"user approval", &review.Attempt{Status: "approved", ApprovalSource: "user-command"}, "Approved"},
		{"model approval", &review.Attempt{Status: "approved", ApprovalSource: "model"}, "Auto-Approved"},
		{"model abstention", &review.Attempt{Status: "auto_reviewed", ApprovalSource: "model"}, "Auto-Reviewed"},
		{"user feedback", &review.Attempt{Status: "feedback", ApprovalSource: "user-command"}, "Feedback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := review.Entry{Stack: stack, Status: "needs_review", Attempt: tc.attempt}
			if tc.attempt != nil {
				entry.Status = tc.attempt.Status
			}
			require.Equal(t, tc.want, EntryStatusLabel(entry))
		})
	}
}
