package timber

import (
	"io"
)

// PruneReporters reports dry-run candidates and completed removals.
type PruneReporters struct {
	DryRun  func(io.Writer, string, string, string) error
	Removal func(io.Writer, string, string, string, bool) error
}
