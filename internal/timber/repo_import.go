package timber

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// trashCommandName is the trash CLI used to move removed paths to the
// system trash instead of deleting them outright.
const trashCommandName = "trash"

// importWorktree is one source worktree that will be recreated under the
// managed Timber layout.
type importWorktree struct {
	Name        string
	BranchName  string // empty for detached worktrees
	CommitHash  string
	CurrentPath string
	TargetPath  string
	Detached    bool
}

// importSkip is a source worktree that cannot be moved; its reason is shown in
// the summary so nothing disappears silently.
type importSkip struct {
	Path   string
	Reason string
}

type repoImportCommandOptions struct {
	runtime Runtime
	name    string
}

func NewRepoImportCommand(runtime Runtime) *cobra.Command {
	options := &repoImportCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:   "import <path>",
		Short: "Convert an existing clone into a managed Timber repository",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}
	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from remote or checkout)")

	return command
}

func (x *repoImportCommandOptions) Execute(command *cobra.Command, args []string) error {
	sourcePath, err := x.runtime.absolutePath(args[0])
	if err != nil {
		return err
	}

	plan, err := x.runtime.buildImportPlan(sourcePath, normalizeRepoName(x.name))
	if err != nil {
		return err
	}
	return plan.run(command)
}

// rejectRegisteredSource refuses to import a repository that is already
// registered so the bare clone cannot shadow an existing registration.
func rejectRegisteredSource(runtime Runtime, commonDir string) error {
	repos, err := runtime.listRegisteredRepos()
	if err != nil {
		return err
	}
	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("repository is already registered as %q", repo.Name)
		}
	}
	return nil
}

func pathIsBareRepository(runtime Runtime, path string) (bool, error) {
	result, err := gitOutput(runtime, path, "rev-parse", "--is-bare-repository")
	if err != nil {
		return false, err
	}
	return result.stdout == "true", nil
}

func shortCommitHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

// isZeroCommitHash reports whether a worktree has no commits yet, where Git
// reports an all-zero HEAD.
func isZeroCommitHash(hash string) bool {
	return strings.Trim(hash, "0") == ""
}

func defaultRepoNameForImport(source *Repository, mainPath string) string {
	if result, err := source.git("remote", "get-url", remoteName); err == nil {
		if name, err := defaultRepoNameFromRemote(result.stdout); err == nil {
			return name
		}
	}
	return defaultRepoNameFromPath(mainPath)
}

func defaultRepoNameFromPath(mainPath string) string {
	return normalizeRepoName(filepath.Base(mainPath))
}

func setupImportedBareOrigin(runtime Runtime, source *Repository, barePath string) error {
	bare, err := openBareRepository(runtime, barePath)
	if err != nil {
		return err
	}

	originURL := ""
	if result, err := source.git("remote", "get-url", remoteName); err == nil {
		originURL = result.stdout
	}

	// Drop the clone-default origin (it points at the ephemeral source checkout).
	_, _ = bare.git("remote", "remove", remoteName)

	if originURL == "" {
		// Local-only source repositories have no origin to track.
		return nil
	}

	if _, err := bare.git("remote", "add", remoteName, originURL); err != nil {
		return err
	}
	return configureBareOriginTracking(runtime, barePath)
}

func ensureBranchUpstream(repository *Repository, branchName string) error {
	_, err := repository.upstreamReference(branchName)
	if err == nil {
		return nil
	}

	upstreamBranch, resolveErr := repository.remoteHeadBranch()
	if resolveErr != nil {
		// Local-only repositories may have no origin; leave upstream unset.
		return nil
	}
	_, err = repository.git("branch", "--set-upstream-to", upstreamBranch, branchName)
	return err
}
