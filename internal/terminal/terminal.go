package terminal

import (
	"io"
	"os"
)

// IsInteractive reports whether the input and controlling terminal are usable.
func IsInteractive(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}
	if fileInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}

	// bubbletea needs /dev/tty; treat its absence as non-interactive.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	if err := tty.Close(); err != nil {
		return false
	}
	return true
}
