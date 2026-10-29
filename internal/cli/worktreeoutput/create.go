package worktreeoutput

import (
	"fmt"
	"io"
)

// ReportCreated writes the worktree creation status.
func ReportCreated(output io.Writer, renderStatus func(string) string, path string) error {
	_, err := fmt.Fprintf(output, "%s\n", renderStatus("created "+path))
	return err
}

// ReportHerdrOpened writes the Herdr workspace status.
func ReportHerdrOpened(output io.Writer, renderStatus func(string) string, name string) error {
	_, err := fmt.Fprintf(output, "%s\n", renderStatus("opened herdr space for "+name))
	return err
}
