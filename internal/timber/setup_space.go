package timber

import (
	"fmt"

	"github.com/spf13/cobra"
)

func reportOpenedHerdrSpace(command *cobra.Command, worktreeName string) error {
	message := fmt.Sprintf("opened herdr space for %s", worktreeName)
	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message))
	return err
}
