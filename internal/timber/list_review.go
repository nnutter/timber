package timber

import (
	"fmt"
)

const reviewLookupConcurrency = 4

// reviewProgress counts commits, including fixups, rather than review stacks.
// A nil progress means the status could not be read, not zero approvals.
type reviewProgress struct {
	Approved int `json:"approved"`
	Total    int `json:"total"`
}

func (x *reviewProgress) String() string {
	if x == nil {
		return "?"
	}
	return fmt.Sprintf("%d/%d", x.Approved, x.Total)
}
