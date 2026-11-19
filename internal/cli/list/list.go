package list

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/timber"
)

// sortValue implements pflag.Value for the list command's sort modes.
type sortValue string

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

func (x *sortValue) Type() string { return "sort" }

const (
	sortRecency  sortValue = "recency"
	sortRepo     sortValue = "repo"
	sortWorktree sortValue = "worktree"
)

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
	List     func(ctx context.Context, stdout io.Writer, options timber.ListOptions) error
	Complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that lists managed worktrees.
func New(deps Dependencies) *cobra.Command {
	var options timber.ListOptions
	sortBy := sortValue(sortRecency)
	command := &cobra.Command{
		Use:               "list [@repo]",
		Aliases:           []string{"ls"},
		Short:             "List managed Git worktrees",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &options, &sortBy),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVar(&options.PullRequests, "pr", false, "Include open pull request status from gh")
	command.Flags().BoolVarP(&options.Todo, "todo", "t", false, "Include TODO progress (implied by --json)")
	command.Flags().BoolVarP(&options.Review, "review", "r", false, "Include commit review progress (implied by --json)")
	command.Flags().BoolVarP(&options.Location, "location", "L", false, "Include worktree location as the last column")
	command.Flags().BoolVar(&options.LocationsOnly, "locations-only", false, "Print only worktree locations, one per line")
	command.Flags().BoolVar(&options.JSON, "json", false, "Output worktrees as JSON instead of a table")
	command.Flags().Var(&sortBy, "sort", "Sort worktrees by recency (newest commit first), repo, or worktree")
	command.MarkFlagsMutuallyExclusive("locations-only", "json")
	command.MarkFlagsMutuallyExclusive("locations-only", "pr")
	command.MarkFlagsMutuallyExclusive("locations-only", "todo")
	command.MarkFlagsMutuallyExclusive("locations-only", "review")
	command.MarkFlagsMutuallyExclusive("locations-only", "location")
	if err := command.RegisterFlagCompletionFunc("sort", completeSort); err != nil {
		panic(err)
	}
	return command
}

func run(deps Dependencies, options *timber.ListOptions, sortBy *sortValue) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		callOptions := *options
		if len(args) == 1 {
			callOptions.RepoArg = args[0]
		}
		callOptions.SortMode = string(*sortBy)
		return deps.List(command.Context(), command.OutOrStdout(), callOptions)
	}
}
