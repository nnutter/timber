package git

// isSame reports whether a and b are equal.
func isSame[T comparable](a, b T) bool {
	return a == b
}
