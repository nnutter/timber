package timber

import (
	"io"

	"github.com/spf13/cobra"

	reviewroot "github.com/nnutter/timber/internal/gitreview/cli/root"
	"github.com/nnutter/timber/internal/gitreview/review"
)

// NewReviewCommand attaches the review workflow to the current checkout.
func NewReviewCommand(runtime Runtime) *cobra.Command {
	var command *cobra.Command
	input := reviewInput{command: &command}
	output := reviewOutput{command: &command}
	terminal := reviewOutput{command: &command, terminal: true}
	command = reviewroot.New(review.Service{Repository: review.Repository{Dir: runtime.CurrentDirectory}}, input, output, terminal, false)
	command.Use = "review"
	command.Args = cobra.NoArgs
	command.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	return command
}

type reviewInput struct{ command **cobra.Command }

func (r reviewInput) Read(p []byte) (int, error) { return r.UnwrapReader().Read(p) }
func (r reviewInput) UnwrapReader() io.Reader    { return (*r.command).InOrStdin() }

type reviewOutput struct {
	command  **cobra.Command
	terminal bool
}

func (w reviewOutput) Write(p []byte) (int, error) { return w.UnwrapWriter().Write(p) }
func (w reviewOutput) UnwrapWriter() io.Writer {
	if w.terminal {
		return (*w.command).ErrOrStderr()
	}
	return (*w.command).OutOrStdout()
}
