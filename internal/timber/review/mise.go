package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// miseConfigFiles names the project configuration files that make mise prompt
// for trust in a checkout root. Trust covers the directory, so one match
// identifies a checkout that needs it.
var miseConfigFiles = []string{"mise.toml", ".mise.toml", "mise.local.toml"}

// mirrorMiseTrust copies the source checkout's mise trust state to the review
// checkout. Git wrappers installed by mise refuse to read an untrusted project
// config, which breaks every Git command in the review checkout.
//
// Call it before resetting the checkout and again after, because paranoid mise
// ties trust to file contents and a reset can change them.
func mirrorMiseTrust(ctx context.Context, source Repository, path string) error {
	if !hasMiseConfig(path) {
		return nil
	}
	trusted, err := miseTrustsDirectory(ctx, source, source.Dir)
	if errors.Is(err, exec.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return setReviewMiseTrust(ctx, source, path, trusted)
}

// setReviewMiseTrust makes the review checkout match the source trust state.
func setReviewMiseTrust(ctx context.Context, source Repository, path string, trusted bool) error {
	current, err := miseTrustsDirectory(ctx, source, path)
	if err != nil {
		return err
	}
	if current == trusted {
		return nil
	}
	args := []string{"trust", path}
	if !trusted {
		args = []string{"trust", "--untrust", path}
	}
	command := exec.CommandContext(ctx, "mise", args...)
	// Run from the source checkout so mise does not read the review config
	// while it changes the trust of that directory.
	command.Dir = source.Dir
	command.Env = slices.Clone(source.environment())
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mirror mise trust in review checkout: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func hasMiseConfig(dir string) bool {
	for _, name := range miseConfigFiles {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// miseTrustsDirectory reports whether mise lists dir as trusted.
func miseTrustsDirectory(ctx context.Context, source Repository, dir string) (bool, error) {
	command := exec.CommandContext(ctx, "mise", "trust", "--show")
	command.Dir = dir
	command.Env = slices.Clone(source.environment())
	output, err := command.Output()
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		return false, fmt.Errorf("inspect mise trust in %s: %w: %s", dir, err, strings.TrimSpace(string(exitError.Stderr)))
	}
	if err != nil {
		return false, fmt.Errorf("inspect mise trust in %s: %w", dir, err)
	}
	return miseListedAsTrusted(string(output), dir), nil
}

// miseListedAsTrusted matches a directory against mise trust --show output,
// which names each config directory as trusted or untrusted.
func miseListedAsTrusted(output, dir string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		name, ok := strings.CutSuffix(strings.TrimSpace(line), ": trusted")
		if !ok {
			continue
		}
		if miseSameDirectory(name, dir) {
			return true
		}
	}
	return false
}

// miseSameDirectory compares a reported path, which mise abbreviates under the
// home directory, to an absolute directory.
func miseSameDirectory(reported, dir string) bool {
	if rest, ok := strings.CutPrefix(reported, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		reported = filepath.Join(home, rest)
	}
	if filepath.Clean(reported) == filepath.Clean(dir) {
		return true
	}
	reportedPath, err := filepath.EvalSymlinks(reported)
	if err != nil {
		return false
	}
	dirPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return reportedPath == dirPath
}
