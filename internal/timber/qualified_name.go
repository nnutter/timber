package timber

import (
	"fmt"
	"strings"
)

type qualifiedName struct {
	Name string
	Repo string
}

func rejectAtInWorktreeName(name string) error {
	if strings.Contains(name, "@") {
		return fmt.Errorf("worktree name %q must not contain @", name)
	}
	return nil
}
