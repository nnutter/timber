package timber

import (
	"fmt"
	"strings"
)

type qualifiedName struct {
	Name string
	Repo string
}

// ValidateWorktreeName rejects names that conflict with repository qualifiers.
func ValidateWorktreeName(name string) error {
	if strings.Contains(name, "@") {
		return fmt.Errorf("worktree name %q must not contain @", name)
	}
	return nil
}
