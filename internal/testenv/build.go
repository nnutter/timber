package testenv

import "os"

// BuildArgs builds a test subprocess. Use an instrumented build only when the
// caller is collecting subprocess counters in GOCOVERDIR.
func BuildArgs(output, pkg string) []string {
	args := []string{"build", "-o", output}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	}
	return append(args, pkg)
}
