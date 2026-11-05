// Package commits reviews feature-branch commit stacks with tuicr.
package commits

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/review/display"
	"github.com/nnutter/timber/internal/timber/review"
)

// Command houses flag values and positional arguments for the commits review.
type Command struct {
	Service  review.Service
	Input    io.Reader
	Output   io.Writer
	Terminal io.Writer
	Stack    string
	Start    string
	End      string
	Session  string
}

// RunE runs the commits review workflow.
func (c *Command) RunE(cmd *cobra.Command, args []string) error {
	options := review.ReviewOptions{Session: c.Session, Start: c.Start, End: c.End}
	if len(args) > 0 {
		options.Stack = args[0]
		c.Stack = args[0]
	}
	return c.runReview(cmd.Context(), options, cmd)
}

func (c *Command) runReview(ctx context.Context, options review.ReviewOptions, cmd *cobra.Command) error {
	asJSON := false
	if flag, err := cmd.Flags().GetBool("json"); err == nil {
		asJSON = flag
	}
	reader, err := review.DefaultSavedReader()
	if err != nil {
		return err
	}
	runner := review.Reviewer{Service: c.Service, Reader: reader, Input: c.Input, Terminal: c.Terminal}
	result, err := runner.Review(ctx, options)
	if err != nil {
		return err
	}
	return display.Write(c.Output, asJSON, result, false)
}

// New builds the commits sub-command. Callers install it onto the parent
// with Install.
func New(svc review.Service, in io.Reader, out, term io.Writer) *cobra.Command {
	c := &Command{Service: svc, Input: in, Output: out, Terminal: term}
	cmd := &cobra.Command{Use: "commits [<commit_stack>]", Aliases: []string{"c", "run", "challenge"}, Short: "Review eligible stacks, or select one stack", Long: "Run tuicr in a stable detached worktree. Without a stack argument, review each\neligible stack in oldest-first order. A commit reference selects one stack. After\napproval or feedback, continue through the selected stacks. With --start or --end,\nlimit selection to the bounded range. Each eligible stack opens once per\ninvocation. A review resumes the attempt that captured the same stack, and it\ncarries the saved review of an unchanged patch onto rewritten commits. Incomplete\nreviews and errors stop the command.", Args: cobra.MaximumNArgs(1), RunE: c.RunE}
	cmd.Flags().StringVar(&c.Start, "start", "", "Review from the stack containing this commit (inclusive)")
	cmd.Flags().StringVar(&c.End, "end", "", "Review through the stack containing this commit (inclusive)")
	cmd.Flags().StringVar(&c.Session, "session", "", "Select an exact saved session ID, slug, or indexed session path")
	return cmd
}

// Install adds the commits sub-command to parent.
func Install(parent *cobra.Command, svc review.Service, in io.Reader, out, term io.Writer) {
	parent.AddCommand(New(svc, in, out, term))
}
