package timber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	reviewroot "github.com/nnutter/timber/internal/cli/review/root"
	"github.com/nnutter/timber/internal/review"
)

// NewReviewCommand attaches the review workflow to the current checkout.
func NewReviewCommand(runtime Runtime) *cobra.Command {
	var command *cobra.Command
	var installSkill, force bool
	input := reviewInput{command: &command}
	output := reviewOutput{command: &command}
	terminal := reviewOutput{command: &command, terminal: true}
	command = reviewroot.New(review.Service{Repository: review.Repository{Dir: runtime.CurrentDirectory}}, input, output, terminal, false)
	command.Use = "review"
	command.Args = cobra.NoArgs
	command.Flags().BoolVar(&installSkill, "install-skill", false, "Install the timber-review skill to ~/.agents/skills")
	command.Flags().BoolVarP(&force, "force", "f", false, "Overwrite an existing installed skill")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if !installSkill {
			if force {
				return fmt.Errorf("--force requires --install-skill")
			}
			return cmd.Help()
		}
		if runtime.ReviewSkillContent == "" {
			return fmt.Errorf("timber-review skill content is not available")
		}
		destination := filepath.Join(runtime.HomeDirectory, ".agents", "skills", "timber-review", "SKILL.md")
		if _, err := os.Stat(destination); err == nil && !force {
			return fmt.Errorf("skill already exists at %s (use --force to overwrite)", destination)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect skill destination %q: %w", destination, err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create skill directory: %w", err)
		}
		if err := os.WriteFile(destination, []byte(runtime.ReviewSkillContent), 0o644); err != nil {
			return fmt.Errorf("write skill file: %w", err)
		}
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", statusStyle.Render("installed timber-review skill to "+destination))
		return err
	}
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
