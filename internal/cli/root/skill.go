package root

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type skillCommandOptions struct {
	homeDirectory string
	content       string
	install       bool
	force         bool
}

func (x *skillCommandOptions) Execute(command *cobra.Command, _ []string) error {
	if !x.install {
		if x.force {
			return fmt.Errorf("--force requires --install-skill")
		}
		return command.Help()
	}
	if x.content == "" {
		return fmt.Errorf("timber skill content is not available")
	}
	destination := filepath.Join(x.homeDirectory, ".agents", "skills", "timber", "SKILL.md")
	if _, err := os.Stat(destination); err == nil && !x.force {
		return fmt.Errorf("skill already exists at %s (use --force to overwrite)", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect skill destination %q: %w", destination, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(destination, []byte(x.content), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("installed timber skill to "+destination))
	return err
}
