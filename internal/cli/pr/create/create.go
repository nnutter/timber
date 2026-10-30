// Package create exposes `timber pr create`.
package create

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Draft holds the pull request values given on the command line.
type Draft struct {
	Title string
	Body  string
	Base  string
	Draft bool
}

// Dependencies connects the create command to pull request creation.
type Dependencies struct {
	Create       func(context.Context, io.Reader, string, Draft) (string, error)
	Complete     func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus func(string) string
}

// New constructs the command that opens a pull request for a worktree.
func New(deps Dependencies) *cobra.Command {
	var draft Draft
	command := &cobra.Command{
		Use:               "create <name[@repo]>",
		Short:             "Create a GitHub pull request for a managed worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              run(deps, &draft),
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().StringVar(&draft.Title, "title", "", "Pull request title")
	command.Flags().StringVar(&draft.Body, "body", "", "Pull request description")
	command.Flags().StringVar(&draft.Base, "base", "", "Branch to merge into (default: the branch's upstream)")
	command.Flags().BoolVar(&draft.Draft, "draft", false, "Open the pull request as a draft")
	// A pull request without a title is never useful, so ask for one rather
	// than letting GitHub reject the request.
	if err := command.MarkFlagRequired("title"); err != nil {
		panic(err)
	}
	return command
}

func run(deps Dependencies, draft *Draft) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, args []string) error {
		var raw string
		if len(args) == 1 {
			raw = args[0]
		}
		url, err := deps.Create(command.Context(), command.InOrStdin(), raw, *draft)
		if err != nil {
			return err
		}
		// The URL is the machine-readable result, so it stays plain on stdout;
		// the styled status goes to stderr like other Timber reports.
		if _, err := fmt.Fprintln(command.OutOrStdout(), url); err != nil {
			return err
		}
		_, err = fmt.Fprintln(command.ErrOrStderr(), deps.RenderStatus("created pull request "+url))
		return err
	}
}
