package timber

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func NewRepoCommand(runtime Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "repo",
		Short: "Manage registered bare repositories",
	}
	command.AddCommand(NewRepoAddCommand(runtime))
	command.AddCommand(NewRepoImportCommand(runtime))
	command.AddCommand(NewRepoListCommand(runtime))
	command.AddCommand(NewRepoRemoveCommand(runtime))
	command.AddCommand(NewRepoRenameCommand(runtime))
	return command
}

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

func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("repository name is required")
	}
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
	for segment := range strings.SplitSeq(name, "/") {
		if segment == "" {
			return fmt.Errorf("repository name %q must not have empty path segments", name)
		}
		if segment == "." || segment == ".." {
			return fmt.Errorf("repository name %q is invalid", name)
		}
	}
	return nil
}
