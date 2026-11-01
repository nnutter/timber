package auto

import "cmp"

// isLess reports whether a sorts before b.
func isLess[T cmp.Ordered](a, b T) bool {
	return a < b
}
