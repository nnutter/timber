package list

import (
	"encoding/json/v2"
	"fmt"
	"io"
)

// JSONWorktree is one worktree in the list JSON output.
type JSONWorktree struct {
	Name        string              `json:"name"`
	Repo        string              `json:"repo"`
	Path        string              `json:"path"`
	Upstream    string              `json:"upstream"`
	Ahead       int                 `json:"ahead"`
	Behind      int                 `json:"behind"`
	Commit      string              `json:"commit"`
	Clean       bool                `json:"clean"`
	Merged      bool                `json:"merged"`
	StatusError bool                `json:"statusError"`
	TodoDone    int                 `json:"todoDone"`
	TodoTotal   int                 `json:"todoTotal"`
	PullRequest string              `json:"pullRequest"`
	Review      *JSONReviewProgress `json:"review,omitzero"`
}

// JSONReviewProgress counts reviewed commits.
type JSONReviewProgress struct {
	Approved int `json:"approved"`
	Total    int `json:"total"`
}

// ReportJSON writes the worktrees as a JSON array.
func ReportJSON(stdout io.Writer, worktrees []JSONWorktree) error {
	if worktrees == nil {
		worktrees = []JSONWorktree{}
	}
	output, err := json.Marshal(worktrees)
	if err != nil {
		return fmt.Errorf("encode worktrees as JSON: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(output))
	return err
}
