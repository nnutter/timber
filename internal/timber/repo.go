package timber

import (
	"fmt"
	"strings"
)

// configureBareOriginTracking makes a bare clone usable like a normal remote-tracking
// repository. `git clone --bare` omits remote.origin.fetch, so refs/remotes/origin/*
// (including origin/HEAD) are never populated without this setup.
func configureBareOriginTracking(runtime Runtime, barePath string) error {
	repository, err := openBareRepository(runtime, barePath)
	if err != nil {
		return err
	}

	if _, err := repository.git(
		"config",
		"remote."+remoteName+".fetch",
		"+refs/heads/*:refs/remotes/"+remoteName+"/*",
	); err != nil {
		return err
	}
	if _, err := repository.git("fetch", remoteName); err != nil {
		return err
	}
	if _, err := repository.git("remote", "set-head", remoteName, "--auto"); err != nil {
		// Non-fatal when the remote has no HEAD; local fallbacks still apply later.
		return nil
	}
	return nil
}

// pushDefaultCurrent makes a plain `git push` send the checked-out branch to
// the branch of the same name on the remote.
const pushDefaultCurrent = "current"

// configurePushDefault keeps `git push` off the branch a worktree tracks.
//
// Every managed worktree tracks the remote head, so Git's default push rule
// would send a feature branch to the default branch. `current` pushes the
// branch to its own name and leaves the upstream alone. The setting is shared
// by the repository, so it covers every worktree at once.
//
// It reports whether it wrote the setting, which lets `repo fix` report the
// repositories it changed.
func configurePushDefault(runtime Runtime, barePath string) (bool, error) {
	repository, err := openBareRepository(runtime, barePath)
	if err != nil {
		return false, err
	}

	value, configured, err := repository.gitConfigValue("push.default")
	if err != nil {
		return false, err
	}
	if configured && isSame(value, pushDefaultCurrent) {
		return false, nil
	}

	if _, err := repository.git("config", "push.default", pushDefaultCurrent); err != nil {
		return false, fmt.Errorf("set push.default to %q: %w", pushDefaultCurrent, err)
	}
	return true, nil
}

func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("repository name is required")
	}
	if err := validateRepoNameCharacters(name); err != nil {
		return err
	}
	return validateRepoNameSegments(name)
}

func validateRepoNameCharacters(name string) error {
	if strings.HasSuffix(name, bareRepoSuffix) {
		return fmt.Errorf("repository name %q must not end with %s", name, bareRepoSuffix)
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("repository name %q must not contain \\ character", name)
	}
	if strings.Contains(name, "@") {
		return fmt.Errorf("repository name %q must not contain @ character", name)
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("repository name %q must not contain : character", name)
	}
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return fmt.Errorf("repository name %q must not have empty path segments", name)
	}
	return nil
}

func validateRepoNameSegments(name string) error {
	for segment := range strings.SplitSeq(name, "/") {
		if segment == "" {
			return fmt.Errorf("repository name %q must not have empty path segments", name)
		}
		if isSame(segment, ".") || isSame(segment, "..") {
			return fmt.Errorf("repository name %q is invalid", name)
		}
	}
	return nil
}
