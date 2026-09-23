package timber

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

type gitCommandOptions struct {
	runtime Runtime
}

func NewGitCommand(runtime Runtime) *cobra.Command {
	options := &gitCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:                "git [<git-args>...]",
		Short:              "Run git with --git-dir set to the current worktree",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE:               options.Execute,
	}

	return command
}

func (x *gitCommandOptions) Execute(command *cobra.Command, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	gitDirResult, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return fmt.Errorf("not inside a worktree: run inside a worktree")
	}

	bareResult, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "rev-parse", "--is-bare-repository")
	if err != nil {
		return err
	}
	if bareResult.stdout == "true" {
		return fmt.Errorf("not inside a worktree: run inside a worktree")
	}
	gitDir := filepath.Clean(gitDirResult.stdout)

	gitArgs := append([]string{"--git-dir", gitDir}, args...)
	gitCommand := x.runtime.command(command.Context(), "git", gitArgs...)
	gitCommand.Dir = x.runtime.CurrentDirectory
	gitCommand.Stdin = command.InOrStdin()
	gitCommand.Stdout = command.OutOrStdout()
	gitCommand.Stderr = command.ErrOrStderr()
	if err := gitCommand.Run(); err != nil {
		return fmt.Errorf("run git: %w", err)
	}
	return nil
}
