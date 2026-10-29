package timber

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type createCommandOptions struct {
	repoSelection
	upstream string
	herdr    bool
	noHerdr  bool
}

func NewCreateCommand(runtime Runtime) *cobra.Command {
	options := &createCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:               "create [name[@repo]] [path]",
		Short:             "Create a managed Git worktree",
		Args:              cobra.MaximumNArgs(2),
		RunE:              options.Execute,
		ValidArgsFunction: runtime.completeCreateArgs,
	}

	command.Flags().StringVarP(&options.upstream, "upstream", "u", "", "Upstream branch")
	command.Flags().BoolVar(&options.herdr, "herdr", false, "Also create a Herdr workspace for the new worktree")
	command.Flags().BoolVar(&options.noHerdr, "no-herdr", false, "Do not create a Herdr workspace")
	command.MarkFlagsMutuallyExclusive("herdr", "no-herdr")

	return command
}

func (x *createCommandOptions) Execute(command *cobra.Command, args []string) error {
	worktreePath, _, err := x.createWorktree(command, args)
	if err != nil {
		return err
	}
	return x.runtime.reportCreatedWorktreePath(command.OutOrStdout(), worktreePath)
}

func (x *createCommandOptions) createWorktree(command *cobra.Command, args []string) (string, bool, error) {
	var raw, explicitPath string
	var hasExplicitPath bool
	if len(args) > 0 {
		raw = args[0]
	}
	if len(args) == 2 {
		explicitPath = args[1]
		hasExplicitPath = true
	}
	return x.runtime.CreateManagedWorktree(
		command.Context(),
		command.InOrStdin(),
		command.ErrOrStderr(),
		x.RepoName,
		raw,
		explicitPath,
		hasExplicitPath,
		x.upstream,
		x.herdr,
		x.noHerdr,
	)
}

// CreateManagedWorktree creates a managed worktree and runs automatic setup.
// It returns the worktree path and whether a Herdr workspace was opened.
// hasExplicitPath selects a custom location; otherwise the managed layout
// is used. An empty explicit path is an error, matching CLI validation.
func (runtime Runtime) CreateManagedWorktree(ctx context.Context, input io.Reader, output io.Writer, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string, herdr, noHerdr bool) (string, bool, error) {
	qualified, err := runtime.parseQualifiedName(raw)
	if err != nil {
		return "", false, err
	}
	if err := rejectAtInWorktreeName(qualified.Name); err != nil {
		return "", false, err
	}
	if qualified.Repo != "" {
		repoName = qualified.Repo
	}

	selection := repoSelection{runtime: runtime, RepoName: repoName}
	repo, repository, err := selection.resolve(input)
	if err != nil {
		return "", false, err
	}

	branchName, err := runtime.resolveCreateWorktreeName(repo.Name, qualified.Name)
	if err != nil {
		return "", false, err
	}

	worktreePath := runtime.managedWorktreePath(repo.Name, branchName)
	if hasExplicitPath {
		if explicitPath == "" {
			return "", false, errors.New("worktree path must not be empty")
		}
		worktreePath, err = runtime.absolutePath(explicitPath)
		if err != nil {
			return "", false, err
		}
	}
	if err := rejectNonEmptyWorktreeDirectory(worktreePath); err != nil {
		return "", false, err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return "", false, err
	}
	if _, err := managedWorktreeByName(worktrees, branchName); err == nil {
		return "", false, fmt.Errorf("worktree %q already exists", branchName)
	}

	if _, err := repository.git("fetch", remoteName); err != nil {
		return "", false, fmt.Errorf("fetch %s: %w", remoteName, err)
	}

	upstreamBranch := upstream
	if upstreamBranch == "" {
		resolvedUpstream, err := repository.remoteHeadBranch()
		if err != nil {
			return "", false, err
		}
		upstreamBranch = resolvedUpstream
	}

	branchExists, err := repository.branchExists(branchName)
	if err != nil {
		return "", false, err
	}
	if err := ensureWorktreeDirectory(worktreePath); err != nil {
		return "", false, err
	}
	if branchExists {
		if _, err := repository.git("worktree", "add", worktreePath, branchName); err != nil {
			return "", false, err
		}
	} else {
		if _, err := repository.git("worktree", "add", "-b", branchName, worktreePath, upstreamBranch); err != nil {
			return "", false, err
		}
	}

	if err := repository.setBranchUpstream(branchName, upstreamBranch); err != nil {
		return "", false, err
	}
	if hasExplicitPath {
		gitDir, err := runtime.worktreeGitDir(worktreePath)
		if err != nil {
			return "", false, err
		}
		if err := os.WriteFile(filepath.Join(gitDir, worktreeNameFile), []byte(branchName+"\n"), 0o600); err != nil {
			return "", false, fmt.Errorf("record worktree name: %w", err)
		}
	}

	if _, err := fmt.Fprintf(output, "%s\n", statusStyle.Render("created "+worktreePath)); err != nil {
		return "", false, err
	}

	runtime.autoTrustMise(ctx, input, output, repository, upstreamBranch, worktreePath)
	runtime.autoInstallHKHooks(ctx, input, output, repository, upstreamBranch, worktreePath)

	if !(herdr || (!noHerdr && runtime.HerdrEnvironment)) {
		return worktreePath, false, nil
	}

	worktree := managedWorktree{
		Repo: repo.Name,
		Name: branchName,
		Path: worktreePath,
	}
	if err := runtime.openHerdrSpace(ctx, worktree); err != nil {
		return "", false, err
	}
	if err := reportOpenedHerdrSpace(output, branchName); err != nil {
		return "", false, err
	}
	return worktreePath, true, nil
}

func rejectNonEmptyWorktreeDirectory(worktreePath string) error {
	info, err := os.Stat(worktreePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect worktree directory %q: %w", worktreePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("cannot create worktree directory %q: a file already exists at that path", worktreePath)
	}
	entries, err := os.ReadDir(worktreePath)
	if err != nil {
		return fmt.Errorf("read worktree directory %q: %w", worktreePath, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("worktree directory %q already exists", worktreePath)
	}
	return nil
}
