package timber

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type gitCommandOptions struct {
	repoSelection
}

func NewGitCommand(runtime Runtime) *cobra.Command {
	options := &gitCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:                "git [worktree[@repo]] [-- git-args...]",
		Short:              "Run git with --git-dir set to a managed worktree",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE:               options.Execute,
		ValidArgsFunction:  runtime.completeQualifiedWorktreeNames,
	}

	return command
}

func (x *gitCommandOptions) Execute(command *cobra.Command, args []string) error {
	targetDirectory := x.runtime.CurrentDirectory
	gitArgs := args
	if len(args) > 0 && args[0] == "--" {
		gitArgs = args[1:]
	} else if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		selected, remaining, ok, err := x.resolveWorktreeSelector(command.InOrStdin(), args)
		if err != nil {
			return err
		}
		if ok {
			targetDirectory = selected
			gitArgs = remaining
			if len(gitArgs) > 0 && gitArgs[0] == "--" {
				gitArgs = gitArgs[1:]
			}
		}
	}

	gitDir, err := gitDirForDirectory(x.runtime, targetDirectory)
	if err != nil {
		return err
	}

	gitArgs = append([]string{"--git-dir", gitDir}, gitArgs...)
	gitCommand := x.runtime.command(command.Context(), "git", gitArgs...)
	gitCommand.Dir = targetDirectory
	gitCommand.Stdin = command.InOrStdin()
	gitCommand.Stdout = command.OutOrStdout()
	gitCommand.Stderr = command.ErrOrStderr()
	if err := gitCommand.Run(); err != nil {
		return fmt.Errorf("run git: %w", err)
	}
	return nil
}

// resolveWorktreeSelector interprets args[0] as a <worktree> or
// <worktree>@<repo> selector like the other worktree commands. A candidate
// containing @ always selects a worktree and returns its errors. A bare
// name selects a worktree only when it resolves to an existing managed
// worktree; otherwise ok is false and the caller treats all args as git
// args so subcommands such as `status` keep working.
func (x *gitCommandOptions) resolveWorktreeSelector(input io.Reader, args []string) (string, []string, bool, error) {
	candidate := args[0]
	if strings.Contains(candidate, "@") {
		_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
		if err != nil {
			return "", nil, false, err
		}
		return worktree.Path, args[1:], true, nil
	}

	repoName, err := x.runtime.inferUniqueRepoForWorktree(candidate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", nil, false, nil
		}
		return "", nil, false, err
	}
	x.RepoName = repoName
	_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
	if err != nil {
		return "", nil, false, err
	}
	return worktree.Path, args[1:], true, nil
}

func gitDirForDirectory(runtime Runtime, directory string) (string, error) {
	gitDirResult, err := gitOutput(runtime, directory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
	}

	bareResult, err := gitOutput(runtime, directory, "rev-parse", "--is-bare-repository")
	if err != nil {
		return "", err
	}
	if bareResult.stdout == "true" {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
	}
	return filepath.Clean(gitDirResult.stdout), nil
}
