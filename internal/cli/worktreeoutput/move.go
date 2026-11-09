package worktreeoutput

import (
	"fmt"
	"io"
)

// ReportMove writes the messages for a moved worktree.
func ReportMove(output io.Writer, renderStatus, renderWarning func(string) string, name, path string, movingCurrentDirectory bool) error {
	if _, err := fmt.Fprintf(output, "%s\n", renderStatus(fmt.Sprintf("moved %s to %s", name, path))); err != nil {
		return err
	}
	if movingCurrentDirectory {
		_, err := fmt.Fprintf(output, "%s\n", renderWarning("current directory moved"))
		return err
	}
	return nil
}
