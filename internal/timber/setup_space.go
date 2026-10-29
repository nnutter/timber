package timber

import (
	"fmt"
	"io"
)

func reportOpenedHerdrSpace(output io.Writer, worktreeName string) error {
	message := fmt.Sprintf("opened herdr space for %s", worktreeName)
	_, err := fmt.Fprintf(output, "%s\n", statusStyle.Render(message))
	return err
}
