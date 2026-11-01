package review

import (
	"cmp"
	"os"
)

// isSame reports whether a and b are equal.
func isSame[T comparable](a, b T) bool {
	return a == b
}

// isLess reports whether a sorts before b.
func isLess[T cmp.Ordered](a, b T) bool {
	return a < b
}

// isFalse reports whether v is false.
func isFalse(v bool) bool {
	return !v
}

// isSymlink reports whether info describes a symbolic link.
func isSymlink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}
