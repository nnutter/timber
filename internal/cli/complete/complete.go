// Package complete adapts worktree and repository queries to Cobra
// shell-completion callbacks. Argument-position rules and completion
// directives live here; data access stays in the application layer.
package complete

import (
	"strings"

	"github.com/spf13/cobra"
)

// Queries supplies the completion candidates.
type Queries interface {
	QueryRegisteredRepoNames(prefix string) ([]string, error)
	QueryRepoSuffixes(worktreeName, repoPrefix string, requireWorktree bool) ([]string, error)
	QueryWorktreeNamesAcrossRepos(prefix string) ([]string, error)
}

func withError(names []string, err error) ([]string, cobra.ShellCompDirective) {
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// QualifiedWorktreeNames completes worktree names and qualified
// worktree@repo selectors.
func QualifiedWorktreeNames(queries Queries) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
			if name == "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return withError(queries.QueryRepoSuffixes(name, repoPrefix, true))
		}
		return withError(queries.QueryWorktreeNamesAcrossRepos(toComplete))
	}
}

// CreateArgs completes the optional name[@repo] argument, then directories
// for the path argument.
func CreateArgs(queries Queries) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 1 {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
		if len(args) > 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
			return withError(queries.QueryRepoSuffixes(name, repoPrefix, false))
		}
		return RepoQualifiers(queries)(nil, args, toComplete)
	}
}

// RepoQualifiers completes @repo selectors.
func RepoQualifiers(queries Queries) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		if toComplete != "" && !strings.HasPrefix(toComplete, "@") {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return withError(queries.QueryRepoSuffixes("", strings.TrimPrefix(toComplete, "@"), false))
	}
}

// RegisteredRepoNames completes registered repository names.
func RegisteredRepoNames(queries Queries) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return withError(queries.QueryRegisteredRepoNames(toComplete))
	}
}

// RepoRenameArguments completes the old repository name, then nothing for
// the new name.
func RepoRenameArguments(queries Queries) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	registered := RegisteredRepoNames(queries)
	return func(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return registered(command, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// RepoSuffix completes worktree@repo selectors for a fixed worktree name.
func RepoSuffix(queries Queries) func(worktreeName, repoPrefix string, requireWorktree bool) ([]string, cobra.ShellCompDirective) {
	return func(worktreeName, repoPrefix string, requireWorktree bool) ([]string, cobra.ShellCompDirective) {
		return withError(queries.QueryRepoSuffixes(worktreeName, repoPrefix, requireWorktree))
	}
}

// RepoInitArguments completes nothing for the name of a repository that does not
// exist yet. The argument is a name, so file completion would only mislead.
func RepoInitArguments() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// InitialBranches completes the conventional names for a repository's first
// branch. A repository that is about to be created has no branches of its own.
func InitialBranches() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	names := []string{"main", "master"}
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		matches := make([]string, 0, len(names))
		for _, name := range names {
			if strings.HasPrefix(name, toComplete) {
				matches = append(matches, name)
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}
}
