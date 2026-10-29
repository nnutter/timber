package timber

import "io"

// RepoRef identifies a registered repository for selection UI.
type RepoRef struct {
	Name     string
	BarePath string
}

// RepoPrompter selects a registered repository interactively.
type RepoPrompter interface {
	Prompt(io.Reader, []RepoRef) (RepoRef, error)
}

func (x registeredRepo) ref() RepoRef {
	return RepoRef(x)
}
