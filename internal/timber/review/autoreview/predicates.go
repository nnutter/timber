package autoreview

import "cmp"

// isEmpty reports whether s is the empty string.
func isEmpty(s string) bool {
	return s == ""
}

// isSame reports whether a and b are equal.
func isSame[T comparable](a, b T) bool {
	return a == b
}

// isLess reports whether a sorts before b.
func isLess[T cmp.Ordered](a, b T) bool {
	return a < b
}

// isTrue reports whether v is true.
func isTrue(v bool) bool {
	return v
}
