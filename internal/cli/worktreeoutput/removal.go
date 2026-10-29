// Package worktreeoutput renders messages shared by worktree commands.
package worktreeoutput

import (
	"fmt"
	"io"
	"os"
)

// ReportRemoval writes the messages for a removed worktree.
func ReportRemoval(output io.Writer, renderStatus, renderWarning func(string) string, name, shortHash, path string, removingCurrentDirectory bool) error {
	if _, err := os.Stat(path); err == nil {
		if _, writeErr := fmt.Fprintf(output, "%s\n", renderWarning("warning: worktree directory still exists: "+path)); writeErr != nil {
			return writeErr
		}
	}
	if _, err := fmt.Fprintf(output, "%s\n", renderStatus(fmt.Sprintf("removed %s at %s", name, shortHash))); err != nil {
		return err
	}
	if removingCurrentDirectory {
		_, err := fmt.Fprintf(output, "%s\n", renderWarning("current directory was removed"))
		return err
	}
	return nil
}
