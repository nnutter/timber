package worktreeoutput

import (
	"fmt"
	"io"
)

// ReportPruneDryRun writes a candidate for pruning.
func ReportPruneDryRun(output io.Writer, renderStatus func(string) string, name, repo, path string) error {
	message := fmt.Sprintf("would prune %s (%s) at %s", name, repo, path)
	_, err := fmt.Fprintf(output, "%s\n", renderStatus(message))
	return err
}
