package cli

import tea "github.com/charmbracelet/bubbletea"

// isEmpty reports whether s is the empty string.
func isEmpty(s string) bool {
	return s == ""
}

// isSame reports whether a and b are equal.
func isSame[T comparable](a, b T) bool {
	return a == b
}

// isTrue reports whether v is true.
func isTrue(v bool) bool {
	return v
}

// isNilCmd reports whether command is a nil command.
func isNilCmd(command tea.Cmd) bool {
	return command == nil
}
