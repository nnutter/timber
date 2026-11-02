package display

// isSame reports whether a and b are equal.
func isSame[T comparable](a, b T) bool {
	return a == b
}

// isFalse reports whether v is false.
func isFalse(v bool) bool {
	return !v
}
