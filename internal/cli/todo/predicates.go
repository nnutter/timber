package todo

import (
	"cmp"
	"os"
)

// isEmpty reports whether s is the empty string.
func isEmpty(s string) bool {
	return s == ""
}

// isFalse reports whether v is false.
func isFalse(v bool) bool {
	return !v
}

// isLess reports whether a sorts before b.
func isLess[T cmp.Ordered](a, b T) bool {
	return a < b
}

// isNotExecutable reports whether info has no execute permission.
func isNotExecutable(info os.FileInfo) bool {
	return info.Mode().Perm()&0o111 == 0
}
