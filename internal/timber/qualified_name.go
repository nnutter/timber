package timber

import (
	"fmt"
	"strings"
)

type qualifiedName struct {
	Name string
	Repo string
}

// isRepoOnly reports whether the selector names a repository and no worktree,
// which is the form that targets repository state such as a version tag.
func (x qualifiedName) isRepoOnly() bool {
	if x.Repo == "" {
		return false
	}
	return x.Name == ""
}

// ValidateWorktreeName rejects names that conflict with repository qualifiers.
func ValidateWorktreeName(name string) error {
	if strings.Contains(name, "@") {
		return fmt.Errorf("worktree name %q must not contain @", name)
	}
	return nil
}
