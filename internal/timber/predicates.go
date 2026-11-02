package timber

import (
	"cmp"
	"os"
	"reflect"
)

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

// isNilFunc reports whether f is a nil function value.
func isNilFunc(f any) bool {
	value := reflect.ValueOf(f)
	return value.Kind() == reflect.Func && value.IsNil()
}

// isSymlink reports whether info describes a symbolic link.
func isSymlink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

// isReservedRepoName reports whether repo cannot name a repository.
func isReservedRepoName(repo string) bool {
	return isEmpty(repo) || isSame(repo, ".") || isSame(repo, "..")
}

// isReservedPathName reports whether name cannot name a path element.
func isReservedPathName(name string) bool {
	return isEmpty(name) || isSame(name, ".") || isSame(name, "/")
}

// isSupportedRemoteScheme reports whether scheme can reach a remote.
func isSupportedRemoteScheme(scheme string) bool {
	return isSame(scheme, "https") || isSame(scheme, "ssh")
}
