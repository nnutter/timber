package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type removeCommandOptions struct {
	repoSelection
	force bool
}

func NewRemoveCommand(runtime Runtime) *cobra.Command {
	options := &removeCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "remove [-f|--force] [name[@repo]]",
		Aliases:           []string{"rm"},
		Short:             "Remove a managed Git worktree",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeQualifiedWorktreeNames,
	}

	command.Flags().BoolVarP(&options.force, "force", "f", false, "Force removal")

	return command
}

func (x *removeCommandOptions) Execute(command *cobra.Command, args []string) error {
	var raw string
	if len(args) == 1 {
		raw = args[0]
	}
	return x.removeWorktree(command, raw, x.force)
}

func (x *removeCommandOptions) removeWorktree(command *cobra.Command, raw string, force bool) error {
	_, repository, worktree, err := x.resolveQualifiedWorktree(command.InOrStdin(), raw)
	if err != nil {
		return err
	}
	name := worktree.Name

	if !force {
		if _, err := repository.git("fetch", remoteName); err != nil {
			return fmt.Errorf("fetch %s: %w", remoteName, err)
		}
	}

	worktree, err = x.runtime.enrichManagedWorktree(repository, worktree)
	if err != nil {
		return err
	}

	if !force && !worktree.Clean {
		return fmt.Errorf("worktree %q is not clean", name)
	}
	if !force && !worktree.Merged {
		return fmt.Errorf("branch %q is not merged to %s", name, shortReference(worktree.UpstreamRef))
	}

	currentDirectory := x.runtime.CurrentDirectory
	removingCurrentDirectory := currentDirectory != "" && pathIsWithin(worktree.Path, currentDirectory)

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return err
	}
	if err := removeEmptyParents(worktree.Path, x.runtime.HomeDirectory); err != nil {
		return err
	}

	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return err
	}
	if branchExists {
		if _, err := repository.git("branch", branchDeleteFlag(force), name); err != nil {
			return err
		}
	}

	if _, err := os.Stat(worktree.Path); err == nil {
		if _, writeErr := fmt.Fprintf(command.ErrOrStderr(), "%s\n", warningStyle.Render("warning: worktree directory still exists: "+worktree.Path)); writeErr != nil {
			return writeErr
		}
	}

	message := fmt.Sprintf("removed %s at %s", name, worktree.shortCommitHash())
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render(message)); err != nil {
		return err
	}
	if removingCurrentDirectory {
		_, err = fmt.Fprintf(
			command.ErrOrStderr(),
			"%s\n",
			warningStyle.Render("current directory was removed"),
		)
		return err
	}
	return nil
}

// removeEmptyParents removes path and empty ancestor directories up to (but not
// including) stopPath or the filesystem root.
func removeEmptyParents(path string, stopPath string) error {
	current := canonicalPath(path)
	stopPath = canonicalPath(stopPath)

	for {
		if current == stopPath || current == string(filepath.Separator) {
			return nil
		}
		if !pathIsWithin(stopPath, current) {
			return nil
		}

		err := os.Remove(current)
		if err == nil {
			current = filepath.Dir(current)
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			current = filepath.Dir(current)
			continue
		}
		if isNotEmptyError(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", current, err)
	}
}
