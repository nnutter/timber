// Package feedback reads saved feedback without launching tuicr.
package feedback

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/review/display"
	"github.com/nnutter/timber/internal/timber/review"
)

// Command houses no flags for the feedback review.
type Command struct {
	Service review.Service
	Output  io.Writer
}

// RunE runs the feedback workflow.
func (c *Command) RunE(cmd *cobra.Command, _ []string) error {
	asJSON := false
	if flag, err := cmd.Flags().GetBool("json"); err == nil {
		asJSON = flag
	}
	result, err := c.Service.Feedback(cmd.Context())
	if err != nil {
		return err
	}
	return display.Write(c.Output, asJSON, result, false)
}

// New builds the feedback sub-command. Callers install it onto the parent
// with Install.
func New(svc review.Service, out io.Writer) *cobra.Command {
	c := &Command{Service: svc, Output: out}
	cmd := &cobra.Command{Use: "feedback", Short: "Read current and historical saved feedback without launching tuicr", Args: cobra.NoArgs, RunE: c.RunE}
	return cmd
}

// Install adds the feedback sub-command to parent.
func Install(parent *cobra.Command, svc review.Service, out io.Writer) {
	parent.AddCommand(New(svc, out))
}
