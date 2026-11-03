// Package bump exposes `timber bump`.
package bump

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	versionbump "github.com/nnutter/timber/internal/timber/bump"
)

// Request holds the bump values given on the command line.
type Request struct {
	Kind    versionbump.Kind
	Pattern string
	NoPush  bool
}

// Dependencies connects the bump command to version tag operations.
type Dependencies struct {
	Bump     func(io.Reader, string, Request) (string, error)
	Complete func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
}

// New constructs the command that bumps the newest semantic version tag.
func New(deps Dependencies) *cobra.Command {
	var options flagOptions
	command := &cobra.Command{
		Use:               "bump (--major|--minor|--patch) [name[@repo]]",
		Short:             "Bump the newest semantic version tag",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &options),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVar(&options.major, "major", false, "Bump the major version")
	command.Flags().BoolVar(&options.minor, "minor", false, "Bump the minor version")
	command.Flags().BoolVar(&options.patch, "patch", false, "Bump the patch version")
	command.Flags().StringVar(&options.pattern, "pattern", "", "Only consider tags matching `git tag -l <pattern>`")
	command.Flags().BoolVar(&options.noPush, "no-push", false, "Do not push the new tag")
	return command
}

// flagOptions holds the flags of one bump command invocation.
type flagOptions struct {
	major   bool
	minor   bool
	patch   bool
	pattern string
	noPush  bool
}

// request validates the flags and turns them into a bump request.
func (options *flagOptions) request() (Request, error) {
	var kind versionbump.Kind
	count := 0
	for _, flag := range []struct {
		set  bool
		kind versionbump.Kind
	}{
		{options.major, versionbump.Major},
		{options.minor, versionbump.Minor},
		{options.patch, versionbump.Patch},
	} {
		if flag.set {
			kind, count = flag.kind, count+1
		}
	}
	if count != 1 {
		return Request{}, errors.New("exactly one of --major, --minor, or --patch is required")
	}
	return Request{Kind: kind, Pattern: options.pattern, NoPush: options.noPush}, nil
}

func run(deps Dependencies, options *flagOptions) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		request, err := options.request()
		if err != nil {
			return err
		}
		var raw string
		if len(args) == 1 {
			raw = args[0]
		}
		tag, err := deps.Bump(command.InOrStdin(), raw, request)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), tag)
		return err
	}
}
