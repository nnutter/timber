package timber

// ListJSONWorktree is one worktree in the list JSON output.
type ListJSONWorktree struct {
	Name        string                  `json:"name"`
	Repo        string                  `json:"repo"`
	Path        string                  `json:"path"`
	Upstream    string                  `json:"upstream"`
	Ahead       int                     `json:"ahead"`
	Behind      int                     `json:"behind"`
	Commit      string                  `json:"commit"`
	Clean       bool                    `json:"clean"`
	Merged      bool                    `json:"merged"`
	StatusError bool                    `json:"statusError"`
	TodoDone    int                     `json:"todoDone"`
	TodoTotal   int                     `json:"todoTotal"`
	PullRequest string                  `json:"pullRequest"`
	Review      *ListJSONReviewProgress `json:"review,omitzero"`
}

// ListJSONReviewProgress counts reviewed commits.
type ListJSONReviewProgress struct {
	Approved int `json:"approved"`
	Total    int `json:"total"`
}

func worktreesToListJSON(worktrees []managedWorktree) []ListJSONWorktree {
	records := make([]ListJSONWorktree, 0, len(worktrees))
	for _, worktree := range worktrees {
		record := ListJSONWorktree{
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
		}
		if worktree.Review != nil {
			record.Review = &ListJSONReviewProgress{
				Approved: worktree.Review.Approved,
				Total:    worktree.Review.Total,
			}
		}
		records = append(records, record)
	}
	return records
}
