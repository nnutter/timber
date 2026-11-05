// Package reset clears the review state of stacks.
package reset

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/review/display"
	"github.com/nnutter/timber/internal/timber/review"
)

// Command houses flag values and positional arguments for the reset review.
type Command struct {
	Service review.Service
	Output  io.Writer
}

// RunE runs the reset workflow.
func (c *Command) RunE(cmd *cobra.Command, args []string) error {
	asJSON := false
	if flag, err := cmd.Flags().GetBool("json"); err == nil {
		asJSON = flag
	}
	selector := ""
	if len(args) > 0 {
		selector = args[0]
	}
	reader, err := review.DefaultSavedReader()
	if err != nil {
		return err
	}
	result, err := c.Service.Reset(cmd.Context(), reader, selector)
	if err != nil {
		return err
	}
	return display.Write(c.Output, asJSON, result, false)
}

// New builds the reset sub-command. Callers install it onto the parent with
// Install.
func New(svc review.Service, out io.Writer) *cobra.Command {
	c := &Command{Service: svc, Output: out}
	return &cobra.Command{Use: "reset [<commit_stack>]", Aliases: []string{"clear"}, Short: "Clear the review state of stacks", Long: "Clear the saved review state without launching tuicr. A commit reference or\ninclusive commit_stack endpoints select one stack, and an invocation without an\nargument clears every stack in the feature range. Saved sessions for the cleared\nstacks are deleted, so tuicr and the agent start from the same empty state.", Args: cobra.MaximumNArgs(1), RunE: c.RunE}
}

// Install adds the reset sub-command to parent.
func Install(parent *cobra.Command, svc review.Service, out io.Writer) {
	parent.AddCommand(New(svc, out))
}
