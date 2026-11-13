package cli

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
