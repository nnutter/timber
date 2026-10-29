package list

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// sortValue implements pflag.Value for the list command's sort modes.
type sortValue string

const (
	sortRecency  sortValue = "recency"
	sortRepo     sortValue = "repo"
	sortWorktree sortValue = "worktree"
)

func (x *sortValue) Set(value string) error {
	switch sortValue(value) {
	case sortRecency, sortRepo, sortWorktree:
		*x = sortValue(value)
		return nil
	default:
		return fmt.Errorf("invalid sort mode %q: expected recency, repo, or worktree", value)
	}
}

func (x *sortValue) String() string { return string(*x) }
func (x *sortValue) Type() string   { return "sort" }

func completeSort(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	var values []string
	for _, mode := range []sortValue{sortRecency, sortRepo, sortWorktree} {
		if strings.HasPrefix(string(mode), prefix) {
			values = append(values, string(mode))
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}

// Dependencies connects the list CLI to worktree listing.
type Dependencies struct {
	List     func(ctx context.Context, stdout io.Writer, repoArg string, pullRequests, location, jsonOutput bool, sortMode string) error
	Complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that lists managed worktrees.
func New(deps Dependencies) *cobra.Command {
	var pullRequests, location, jsonOutput bool
	sortBy := sortValue(sortRecency)
	command := &cobra.Command{
		Use:               "list [@repo]",
		Aliases:           []string{"ls"},
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &pullRequests, &location, &jsonOutput, &sortBy),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVar(&pullRequests, "pr", false, "Include open pull request status from gh")
	command.Flags().BoolVarP(&location, "location", "L", false, "Include worktree location as the last column")
	command.Flags().BoolVar(&jsonOutput, "json", false, "Output worktrees as JSON instead of a table")
	command.Flags().Var(&sortBy, "sort", "Sort worktrees by recency (newest commit first), repo, or worktree")
	if err := command.RegisterFlagCompletionFunc("sort", completeSort); err != nil {
		panic(err)
	}
	return command
}

func run(deps Dependencies, pullRequests, location, jsonOutput *bool, sortBy *sortValue) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		var repoArg string
		if len(args) == 1 {
			repoArg = args[0]
		}
		return deps.List(command.Context(), command.OutOrStdout(), repoArg, *pullRequests, *location, *jsonOutput, string(*sortBy))
	}
}
