package timber

import (
	"io"
)

// ListRenderers provides the output functions used by ListWorktrees.
type ListRenderers struct {
	Table func(io.Writer, []ListWorktreeView, bool, bool) error
	JSON  func(io.Writer, []ListJSONWorktree) error
}
