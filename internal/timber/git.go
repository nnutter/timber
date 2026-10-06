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
		Use:                "git [worktree[@repo]|@repo] [-- git-args...]",
		Short:              "Run git with --git-dir set to a managed worktree or repository",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE:               options.Execute,
		ValidArgsFunction:  options.completeSelector,
	}

	return command
}

func (x *gitCommandOptions) Execute(command *cobra.Command, args []string) error {
	targetDirectory := x.runtime.CurrentDirectory
	explicitGitDir := ""
	gitArgs := args
	if len(args) > 0 && args[0] == "--" {
		gitArgs = args[1:]
	} else if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		selectedDir, selectedGitDir, remaining, ok, err := x.resolveWorktreeSelector(command.InOrStdin(), args)
		if err != nil {
			return err
		}
		if ok {
			targetDirectory = selectedDir
			explicitGitDir = selectedGitDir
			gitArgs = remaining
			if len(gitArgs) > 0 && gitArgs[0] == "--" {
				gitArgs = gitArgs[1:]
			}
		}
	}

	gitDir := explicitGitDir
	if gitDir == "" {
		resolved, err := gitDirForDirectory(x.runtime, targetDirectory)
		if err != nil {
			return err
		}
		gitDir = resolved
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

// resolveWorktreeSelector interprets args[0] as a <worktree>, <worktree>@<repo>,
// or @<repo> selector. A candidate containing @ always selects a target and
// returns its errors: a bare worktree name selects the managed worktree, while
// a repo-only @<repo> selects the bare repository. A bare name selects a
// worktree only when it resolves to an existing managed worktree; otherwise
// ok is false and the caller treats all args as git args so subcommands such
// as `status` keep working. The returned gitDir is set for the repo-only
// case; it is empty for worktrees so the caller resolves it from the
// worktree directory.
func (x *gitCommandOptions) resolveWorktreeSelector(input io.Reader, args []string) (string, string, []string, bool, error) {
	candidate := args[0]
	if strings.Contains(candidate, "@") {
		qualified, err := x.runtime.parseQualifiedName(candidate)
		if err != nil {
			return "", "", nil, false, err
		}
		if qualified.Name == "" {
			repo, err := x.runtime.registeredRepoByName(qualified.Repo)
			if err != nil {
				return "", "", nil, false, err
			}
			return repo.BarePath, repo.BarePath, args[1:], true, nil
		}
		_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
		if err != nil {
			return "", "", nil, false, err
		}
		return worktree.Path, "", args[1:], true, nil
	}

	repoName, err := x.runtime.inferUniqueRepoForWorktree(candidate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", nil, false, nil
		}
		return "", "", nil, false, err
	}
	x.RepoName = repoName
	_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
	if err != nil {
		return "", "", nil, false, err
	}
	return worktree.Path, "", args[1:], true, nil
}

func (x *gitCommandOptions) completeSelector(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if name, repoPrefix, found := strings.CutLast(toComplete, "@"); found {
		if name == "" {
			return x.runtime.completeRepoSuffix("", repoPrefix, false)
		}
		return x.runtime.completeRepoSuffix(name, repoPrefix, true)
	}
	if toComplete != "" && !strings.HasPrefix(toComplete, "@") {
		return x.runtime.completeQualifiedWorktreeNames(command, args, toComplete)
	}
	worktrees, _ := x.runtime.completeQualifiedWorktreeNames(command, args, toComplete)
	repos, _ := x.runtime.completeRepoSuffix("", strings.TrimPrefix(toComplete, "@"), false)
	return append(worktrees, repos...), cobra.ShellCompDirectiveNoFileComp
}

func gitDirForDirectory(runtime Runtime, directory string) (string, error) {
	gitDirResult, err := gitOutput(runtime, directory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or @<repo> or run inside a worktree")
	}

	bareResult, err := gitOutput(runtime, directory, "rev-parse", "--is-bare-repository")
	if err != nil {
		return "", err
	}
	if bareResult.stdout == "true" {
		return "", fmt.Errorf("not inside a worktree: pass a worktree name or @<repo> or run inside a worktree")
	}
	return filepath.Clean(gitDirResult.stdout), nil
}
