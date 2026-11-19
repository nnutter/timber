package list

import (
	"fmt"
	"io"
)

// ReportLocations writes one worktree location per line to stdout.
func ReportLocations(stdout io.Writer, locations []string) error {
	for _, location := range locations {
		if _, err := fmt.Fprintln(stdout, location); err != nil {
			return err
		}
	}
	return nil
}
