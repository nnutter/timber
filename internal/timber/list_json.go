package timber

import (
	"encoding/json/v2"
	"fmt"
	"io"
)

// listJSONWorktree is the JSON record for one worktree in `list --json`
// output. It carries the same information as a table row: identity,
// upstream divergence, TODO.md progress, full commit hash, dirtiness,
// merge state, optional commit review progress, and the pull request display
// string when --pr is used. Review is absent when its status cannot be read.
// Worktrees whose git status could not be read report statusError instead
// of divergence data.
type listJSONWorktree struct {
	Name        string          `json:"name"`
	Repo        string          `json:"repo"`
	Path        string          `json:"path"`
	Upstream    string          `json:"upstream"`
	Ahead       int             `json:"ahead"`
	Behind      int             `json:"behind"`
	Commit      string          `json:"commit"`
	Clean       bool            `json:"clean"`
	Merged      bool            `json:"merged"`
	StatusError bool            `json:"statusError"`
	TodoDone    int             `json:"todoDone"`
	TodoTotal   int             `json:"todoTotal"`
	PullRequest string          `json:"pullRequest"`
	Review      *reviewProgress `json:"review,omitzero"`
}

func worktreesToListJSON(worktrees []managedWorktree) []listJSONWorktree {
	records := make([]listJSONWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		records = append(records, listJSONWorktree{
			Name:        worktree.Name,
			Repo:        worktree.Repo,
			Path:        worktree.Path,
			Upstream:    worktree.ListStatus.Upstream,
			Ahead:       worktree.ListStatus.Ahead,
			Behind:      worktree.ListStatus.Behind,
			Commit:      worktree.CommitHash,
			Clean:       worktree.Clean,
			Merged:      worktree.Merged,
			StatusError: worktree.ListError,
			TodoDone:    worktree.Todo.Done,
			TodoTotal:   worktree.Todo.Total,
			PullRequest: worktree.PullRequest,
			Review:      worktree.Review,
		})
	}
	return records
}

func reportListJSON(stdout io.Writer, worktrees []managedWorktree) error {
	output, err := json.Marshal(worktreesToListJSON(worktrees))
	if err != nil {
		return fmt.Errorf("encode worktrees as JSON: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(output))
	return err
}
