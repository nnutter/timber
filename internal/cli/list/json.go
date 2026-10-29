package list

import (
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/nnutter/timber/internal/timber"
)

// ReportJSON writes the worktrees as a JSON array.
func ReportJSON(stdout io.Writer, worktrees []timber.ListJSONWorktree) error {
	if worktrees == nil {
		worktrees = []timber.ListJSONWorktree{}
	}
	output, err := json.Marshal(worktrees)
	if err != nil {
		return fmt.Errorf("encode worktrees as JSON: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(output))
	return err
}
