package timber

import (
	"fmt"
	"io"

	"github.com/nnutter/timber/internal/display"
)

func reportOpenedHerdrSpace(output io.Writer, worktreeName string) error {
	message := fmt.Sprintf("opened herdr space for %s", worktreeName)
	_, err := fmt.Fprintf(output, "%s\n", display.StatusStyle.Render(message))
	return err
}
