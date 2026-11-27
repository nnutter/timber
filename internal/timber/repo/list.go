// Package repo defines the data returned by the repository list query.
package repo

import "github.com/nnutter/timber/internal/timber/todo"

// Repo contains the information displayed for a registered repository.
type Repo struct {
	Name   string
	Alias  string
	Origin string
	Todo   todo.Progress
}
