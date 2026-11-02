package git

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// Dependencies connects the Git CLI to repository selection and process state.
type Dependencies struct {
	CurrentDirectory   string
	Command            func(context.Context, string, ...string) *exec.Cmd
	ResolveSelector    func(io.Reader, string) (directory, gitDir string, selected bool, err error)
	ResolveGitDir      func(string) (string, error)
	CompleteWorktrees  func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	CompleteRepoSuffix func(string, string, bool) ([]string, cobra.ShellCompDirective)
}

type gitCommandOptions struct{ Dependencies }

func (x *gitCommandOptions) Execute(command *cobra.Command, args []string) error {
	targetDirectory := x.CurrentDirectory
	explicitGitDir := ""
	gitArgs := args
	if len(args) > 0 && isSame(args[0], "--") {
		gitArgs = args[1:]
	} else if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		var err error
		targetDirectory, explicitGitDir, gitArgs, err = x.selectedGitTarget(command, args)
		if err != nil {
			return err
		}
	}
	return x.runGit(command, targetDirectory, explicitGitDir, gitArgs)
}

func (x *gitCommandOptions) completeSelector(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
		if name == "" {
			return x.CompleteRepoSuffix("", repoPrefix, false)
		}
		return x.CompleteRepoSuffix(name, repoPrefix, true)
	}
	if toComplete != "" && !strings.HasPrefix(toComplete, "@") {
		return x.CompleteWorktrees(command, args, toComplete)
	}
	worktrees, _ := x.CompleteWorktrees(command, args, toComplete)
	repos, _ := x.CompleteRepoSuffix("", strings.TrimPrefix(toComplete, "@"), false)
	return append(worktrees, repos...), cobra.ShellCompDirectiveNoFileComp
}

func (x *gitCommandOptions) runGit(command *cobra.Command, targetDirectory, explicitGitDir string, gitArgs []string) error {
	gitDir := explicitGitDir
	if gitDir == "" {
		resolved, err := x.ResolveGitDir(targetDirectory)
		if err != nil {
			return err
		}
		gitDir = resolved
	}
	gitArgs = append([]string{"--git-dir", gitDir}, gitArgs...)
	gitCommand := x.Command(command.Context(), "git", gitArgs...)
	gitCommand.Dir = targetDirectory
	gitCommand.Stdin = command.InOrStdin()
	gitCommand.Stdout = command.OutOrStdout()
	gitCommand.Stderr = command.ErrOrStderr()
	if err := gitCommand.Run(); err != nil {
		return fmt.Errorf("run git: %w", err)
	}
	return nil
}

func (x *gitCommandOptions) selectedGitTarget(command *cobra.Command, args []string) (string, string, []string, error) {
	selectedDir, selectedGitDir, ok, err := x.ResolveSelector(command.InOrStdin(), args[0])
	if err != nil {
		return "", "", nil, err
	}
	if !ok {
		return x.CurrentDirectory, "", args, nil
	}
	gitArgs := args[1:]
	if len(gitArgs) > 0 && isSame(gitArgs[0], "--") {
		gitArgs = gitArgs[1:]
	}
	return selectedDir, selectedGitDir, gitArgs, nil
}

// New constructs the Git pass-through command.
func New(dependencies Dependencies) *cobra.Command {
	options := &gitCommandOptions{Dependencies: dependencies}
	return &cobra.Command{
		Use:                "git [worktree[@repo]|@repo] [-- git-args...]",
		Short:              "Run git with --git-dir set to a managed worktree or repository",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE:               options.Execute,
		ValidArgsFunction:  options.completeSelector,
	}
}
