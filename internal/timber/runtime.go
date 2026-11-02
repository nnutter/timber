package timber

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/go-github/v91/github"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	coreconfig "github.com/nnutter/timber/internal/timber/config"
	repomodel "github.com/nnutter/timber/internal/timber/repo"
	"github.com/nnutter/timber/internal/timber/review"
	"github.com/nnutter/timber/internal/timber/todo"
)

const (
	createPathFileEnvVarName = "TIMBER_CREATE_PATH_FILE"
	worktreesDirName         = "worktrees"
	worktreeRootEnvVarName   = "TIMBER_WORKTREE_ROOT"
)

// Runtime contains the process state and operating-system settings used by a
// timber command. A Runtime is captured once at the CLI boundary and should
// be treated as immutable for the lifetime of a command.
type Runtime struct {
	CurrentDirectory string

	HomeDirectory      string
	DataHome           string
	ConfigHome         string
	WorktreeRoot       string
	TemporaryDirectory string

	HerdrEnvironment bool

	// RepoPrompter selects a repository when interactive resolution is needed.
	RepoPrompter RepoPrompter

	CreatePathFile string
	SwitchPathFile string
	RenamePathFile string

	// Environment is passed to child processes such as git and herdr.
	Environment []string

	// HerdrExecutable overrides the executable used for Herdr commands. It is
	// primarily useful for tests; an empty value uses PATH lookup.
	HerdrExecutable string

	// GhExecutable overrides the executable used for GitHub commands. It is
	// primarily useful for tests; an empty value uses PATH lookup.
	GhExecutable string

	// GhConfigDirectory overrides where the GitHub CLI keeps its
	// configuration, which Timber reads for a stored credential. An empty
	// value follows GH_CONFIG_DIR and then the Timber config home.
	GhConfigDirectory string

	// GitHubAPIBaseURL overrides the GitHub API endpoint. It is primarily
	// useful for tests; an empty value uses the github.com endpoint.
	GitHubAPIBaseURL string

	// GitHubKeyringLookup overrides the system keyring read used to find a
	// GitHub CLI credential. It is primarily useful for tests; an empty value
	// uses the system keyring.
	GitHubKeyringLookup func(service, user string) (string, error)

	// TrashExecutable overrides the executable used to move removed paths to
	// the system trash. It is primarily useful for tests; an empty value uses
	// PATH lookup of "trash".
	TrashExecutable string

	// TodoSkillContent holds the embedded skills/timber-todo/SKILL.md
	// installed by `timber todo --install-skill`. It is set by main from
	// the embedded file; an empty value reports an error instead of
	// writing an empty skill.
	TodoSkillContent string

	// SkillContent holds skills/timber/SKILL.md, embedded by main and
	// installed by `timber --install-skill`.
	SkillContent string

	// ReviewSkillContent is installed by `timber review --install-skill`.
	ReviewSkillContent string
}

// AddRepo registers a bare repository from a remote URL or path. It
// returns the repository name and target path for CLI reporting.
func (runtime Runtime) AddRepo(source, name, alias string) (string, string, error) {
	remoteURL, err := resolveRemoteURL(source)
	if err != nil {
		return "", "", err
	}
	repoName, err := repoNameForAdd(name, remoteURL)
	if err != nil {
		return "", "", err
	}
	targetPath := runtime.bareRepoPath(repoName)
	if err := rejectExistingRepo(targetPath, repoName); err != nil {
		return "", "", err
	}
	if err := cloneBareRepo(runtime, remoteURL, targetPath, alias); err != nil {
		return "", "", err
	}
	return repoName, targetPath, nil
}

// ConfigureWorktreeHooks moves one worktree to its own Git hooks directory so
// that every worktree can carry different hooks. It copies the hooks the
// worktree had before the split.
//
// Timber sets core.hooksPath only when no scope sets it already. A hooks path
// from the person's environment, a hook manager such as hk, or an earlier
// Timber run therefore stays in control.
func (runtime Runtime) ConfigureWorktreeHooks(worktreePath string) error {
	configured, err := gitSettingIsSet(runtime, worktreePath, "core.hooksPath")
	if err != nil {
		return err
	}
	if configured {
		return nil
	}

	gitDir, err := runtime.worktreeGitDir(worktreePath)
	if err != nil {
		return err
	}
	commonDirResult, err := gitOutput(runtime, worktreePath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("resolve common Git directory for %q: %w", worktreePath, err)
	}
	commonDir := filepath.Clean(commonDirResult.stdout)

	if err := runtime.enableWorktreeConfig(commonDir); err != nil {
		return err
	}

	hooksPath := filepath.Join(gitDir, "hooks")
	if err := cloneHooksDirectory(filepath.Join(commonDir, "hooks"), hooksPath); err != nil {
		return err
	}
	if _, err := gitOutput(runtime, worktreePath, "config", "--worktree", "core.hooksPath", hooksPath); err != nil {
		return fmt.Errorf("set per-worktree hooks path: %w", err)
	}
	return nil
}

// CreateManagedWorktree creates a managed worktree and runs automatic setup.
// It returns the worktree path and whether a Herdr workspace was opened.
// hasExplicitPath selects a custom location; otherwise the managed layout
// is used. An empty explicit path is an error, matching CLI validation.
func (runtime Runtime) CreateManagedWorktree(ctx context.Context, input io.Reader, output io.Writer, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string, herdr, noHerdr bool, reporters CreateReporters) (string, bool, error) {
	if err := validateCreateReporters(runtime, herdr, noHerdr, reporters); err != nil {
		return "", false, err
	}
	created, err := createWorktreeCheckout(runtime, input, repoName, raw, explicitPath, hasExplicitPath, upstream)
	if err != nil {
		return "", false, err
	}
	if err := reporters.Created(output, created.path); err != nil {
		return "", false, err
	}
	runtime.autoTrustMise(ctx, input, output, created.repository, created.upstream, created.path, reporters.SetupPrompt, reporters.SetupWarning)
	runtime.autoInstallHKHooks(ctx, input, output, created.repository, created.upstream, created.path, reporters.SetupPrompt, reporters.SetupWarning)
	// Run after hook installation so the worktree hooks directory keeps the
	// hooks a hook manager just installed.
	if err := runtime.ConfigureWorktreeHooks(created.path); err != nil {
		return "", false, err
	}
	return openCreatedHerdr(ctx, runtime, output, created, herdr, noHerdr, reporters)
}

// CreatePullRequest opens a GitHub pull request for a managed worktree and
// returns its browser URL.
//
// The command only creates the pull request. It never pushes, so a branch that
// is missing from GitHub or behind the worktree fails with the push command to
// run. Only github.com is supported.
//
// Every request names the base repository and the base branch. GitHub otherwise
// chooses a base, and from a fork it chooses the fork's source repository.
func (runtime Runtime) CreatePullRequest(ctx context.Context, input io.Reader, draft PullRequestDraft) (string, error) {
	if strings.TrimSpace(draft.Title) == "" {
		return "", fmt.Errorf("a pull request needs a title")
	}
	target, err := runtime.pullRequestTarget(input, draft)
	if err != nil {
		return "", err
	}

	token, err := runtime.githubToken()
	if err != nil {
		return "", err
	}
	client, err := runtime.newGitHubClient(token)
	if err != nil {
		return "", err
	}

	if err := target.requirePushed(ctx, client); err != nil {
		return "", err
	}

	request := github.CreatePullRequest{
		Title: new(draft.Title),
		Head:  target.head,
		Base:  target.base,
		Draft: new(draft.Draft),
	}
	if draft.Body != "" {
		request.Body = new(draft.Body)
	}
	pullRequest, _, err := client.PullRequests.Create(ctx, target.owner, target.repo, request)
	if err != nil {
		return "", fmt.Errorf("create pull request from %s into %s: %w", target.head, target.base, err)
	}
	if url := pullRequest.GetHTMLURL(); url != "" {
		return url, nil
	}
	return "", fmt.Errorf("GitHub created a pull request without a URL")
}

// DefineHerdrSpace defines Herdr tabs in the current space for the
// identified worktree.
func (x Runtime) DefineHerdrSpace(ctx context.Context, repo, name, path string) error {
	return x.defineCurrentHerdrSpace(ctx, managedWorktree{Repo: repo, Name: name, Path: path})
}

// FixRepos moves legacy bare repositories into the worktree root and
// repairs stale worktrees. It returns one status message per action for CLI
// reporting.
func (runtime Runtime) FixRepos() ([]string, error) {
	var messages []string
	legacyRepos, err := runtime.listLegacyRepos()
	if err != nil {
		return nil, err
	}
	for _, repo := range legacyRepos {
		moved, err := runtime.moveLegacyRepo(repo)
		if err != nil {
			return nil, err
		}
		if !moved {
			continue
		}
		messages = append(messages, "moved repository "+repo.Name)
	}

	repos, err := runtime.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		repaired, err := runtime.repairStaleWorktrees(repo)
		if err != nil {
			return nil, err
		}
		if !repaired {
			continue
		}
		messages = append(messages, "repaired worktrees for "+repo.Name)
	}
	return messages, nil
}

// GitDirForDirectory resolves a worktree Git directory and rejects bare repositories.
func (runtime Runtime) GitDirForDirectory(directory string) (string, error) {
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

// HerdrConfigFile is the Herdr configuration file that lists keybindings.
func (x Runtime) HerdrConfigFile() string {
	return x.herdrConfigFilePath()
}

// HerdrPluginDirectory is the directory the Herdr plugin is installed to.
func (x Runtime) HerdrPluginDirectory() string {
	return x.herdrPluginInstallDirectory()
}

// ImportRepo converts an existing clone into a managed Timber repository.
// Both reporter callbacks are required.
func (runtime Runtime) ImportRepo(source, name string, reporter ImportReporter) error {
	if reporter.Registered == nil || isNilFunc(reporter.Summary) {
		return errors.New("repository import requires registration and summary reporters")
	}
	sourcePath, err := runtime.absolutePath(source)
	if err != nil {
		return err
	}

	plan, err := runtime.buildImportPlan(sourcePath, normalizeRepoName(name))
	if err != nil {
		return err
	}
	return plan.run(reporter)
}

// InitRepo creates an empty bare repository under the worktree root. It
// returns the repository name and Git directory for CLI reporting.
//
// The repository has no remote. Its initial branch carries one empty commit so
// managed worktrees can branch from it before the first push.
func (runtime Runtime) InitRepo(name, initialBranch string) (string, string, error) {
	repoName := normalizeRepoName(name)
	if err := validateRepoName(repoName); err != nil {
		return "", "", err
	}

	branch := cmp.Or(strings.TrimSpace(initialBranch), defaultInitialBranch)
	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "check-ref-format", "--branch", branch); err != nil {
		return "", "", fmt.Errorf("invalid initial branch %q: %w", branch, err)
	}

	targetPath := runtime.bareRepoPath(repoName)
	if _, err := os.Stat(targetPath); err == nil {
		return "", "", fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}

	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return "", "", err
	}

	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "init", "--bare", targetPath); err != nil {
		return "", "", err
	}

	if err := initializeBranch(runtime, targetPath, branch); err != nil {
		// A repository without its initial commit cannot serve worktrees and
		// would block a retry by existing, so discard the half-made repository.
		removeFailedInit(targetPath, runtime.worktreeRoot())
		return "", "", err
	}

	return repoName, targetPath, nil
}

// LinkHerdrPlugin links the installed Herdr plugin.
func (x Runtime) LinkHerdrPlugin(ctx context.Context, destination string) error {
	_, err := x.runHerdr(ctx, "plugin", "link", destination, "--enabled")
	return err
}

// ListRepos returns registered repositories in name order. When details is
// false, it avoids reading each repository's Git configuration.
func (x Runtime) ListRepos(details bool) ([]repomodel.Repo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	result := make([]repomodel.Repo, 0, len(repos))
	for _, repo := range repos {
		item := repomodel.Repo{Name: repo.Name}
		if details {
			item.Origin = repo.originURL(x)
			item.Alias = repo.alias(x, item.Origin)
		}
		result = append(result, item)
	}
	return result, nil
}

// ListWorktrees lists managed worktrees as a table or JSON document. All
// output is written to the given writers, preserving CLI reporting order.
func (runtime Runtime) ListWorktrees(ctx context.Context, stdout io.Writer, repoArg string, pullRequests, location, jsonOutput bool, sortMode string, renderers ListRenderers) error {
	var sortBy listSort
	if err := sortBy.Set(sortMode); err != nil {
		return err
	}

	selection := repoSelection{runtime: runtime}
	if repoArg != "" {
		repo, err := runtime.parseRepoOnlyArg(repoArg)
		if err != nil {
			return err
		}
		selection.RepoName = repo
	}

	repos, err := selection.reposToConsider()
	if err != nil {
		return err
	}
	enrich := func(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
		return enrichWorktreeForSort(runtime, repository, worktree, sortBy)
	}
	worktrees, err := runtime.collectWorktrees(repos, enrich)
	if err != nil {
		return err
	}
	sortBy.sort(worktrees)
	runtime.enrichListedWorktreesWithReviews(ctx, worktrees)
	if err := attachPullRequests(runtime, ctx, worktrees, pullRequests); err != nil {
		return err
	}
	return renderWorktreeList(runtime, stdout, worktrees, pullRequests, location, jsonOutput, renderers)
}

// LoadConfig loads the persistent Timber settings.
func (x Runtime) LoadConfig() (coreconfig.Settings, error) {
	return x.loadConfig()
}

// NewCommand builds an external command with the runtime environment and
// executable overrides applied. It exposes process creation for CLI wiring.
func (x Runtime) NewCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return x.command(ctx, name, args...)
}

// OpenHerdrSpace opens a Herdr workspace for the identified worktree.
func (x Runtime) OpenHerdrSpace(ctx context.Context, repo, name, path string) error {
	return x.openHerdrSpace(ctx, managedWorktree{Repo: repo, Name: name, Path: path})
}

// PruneWorktrees removes merged and clean managed worktrees. With prompt it
// asks which worktrees to remove; with dryRun it only reports candidates.
// A non-nil prompter is required when prompt is true.
// All progress is written to output, preserving CLI reporting order.
func (runtime Runtime) PruneWorktrees(input io.Reader, output io.Writer, repoArg string, prompt, dryRun bool, prompter WorktreePrompter, reporters PruneReporters) error {
	if err := validatePruneReporters(prompt, dryRun, prompter, reporters); err != nil {
		return err
	}
	enrichedWorktrees, err := pruneCandidates(runtime, repoArg)
	if err != nil {
		return err
	}
	selectedWorktrees, err := selectPruneWorktrees(input, output, enrichedWorktrees, prompt, prompter)
	if err != nil {
		return err
	}
	return finishPrune(runtime, output, selectedWorktrees, prompt, dryRun, reporters)
}

// QueryRegisteredRepoNames lists registered repository names with the
// given prefix for shell completion.
func (x Runtime) QueryRegisteredRepoNames(toComplete string) ([]string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		if strings.HasPrefix(repo.Name, toComplete) {
			names = append(names, repo.Name)
		}
	}
	return names, nil
}

// QueryRepoSuffixes lists worktree@repo candidates for shell completion.
func (x Runtime) QueryRepoSuffixes(worktreeName string, repoPrefix string, requireWorktree bool) ([]string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, repo := range repos {
		if !strings.HasPrefix(repo.Name, repoPrefix) {
			continue
		}
		if requireWorktree {
			worktrees, err := x.worktreesForRepo(repo)
			if err != nil {
				return nil, err
			}
			if _, err := managedWorktreeByName(worktrees, worktreeName); err != nil {
				continue
			}
		}
		names = append(names, worktreeName+"@"+repo.Name)
	}
	slices.Sort(names)
	return names, nil
}

// QueryWorktreeNamesAcrossRepos lists worktree names for shell completion,
// qualifying ambiguous names with their repository.
func (x Runtime) QueryWorktreeNamesAcrossRepos(toComplete string) ([]string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}

	reposForName := make(map[string][]string)
	var names []string
	for _, repo := range repos {
		worktrees, err := x.worktreesForRepo(repo)
		if err != nil {
			return nil, err
		}
		for _, worktree := range worktrees {
			name := worktree.Name
			if !strings.HasPrefix(name, toComplete) {
				continue
			}
			if _, exists := reposForName[name]; !exists {
				names = append(names, name)
			}
			reposForName[name] = append(reposForName[name], repo.Name)
		}
	}

	completions := make([]string, 0, len(names))
	for _, name := range names {
		reposWithName := reposForName[name]
		if len(reposWithName) == 1 {
			completions = append(completions, name)
			continue
		}
		for _, repoName := range reposWithName {
			completions = append(completions, name+"@"+repoName)
		}
	}
	slices.Sort(completions)
	return completions, nil
}

func (runtime Runtime) RemoveRepo(repoName string) error {
	repository, repo, err := runtime.openRegisteredRepository(repoName)
	if err != nil {
		return err
	}

	worktrees, err := runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return err
	}
	if len(worktrees) > 0 {
		names := make([]string, 0, len(worktrees))
		for _, worktree := range worktrees {
			names = append(names, worktree.Name)
		}
		return fmt.Errorf(
			"repository %q still has managed worktrees (%s); remove them first",
			repoName,
			strings.Join(names, ", "),
		)
	}

	// Also refuse if git still has any non-bare linked worktrees (unmanaged).
	linked, err := linkedWorktreeCount(repository, repo.BarePath)
	if err != nil {
		return err
	}
	if linked > 0 {
		return fmt.Errorf("repository %q still has %d linked worktree(s); remove them first", repoName, linked)
	}

	if err := os.RemoveAll(repo.BarePath); err != nil {
		return fmt.Errorf("remove bare repository %q: %w", repo.BarePath, err)
	}
	if err := runtime.removeEmptyBareParents(repo.BarePath); err != nil {
		return err
	}

	return nil
}

// RemoveWorktree resolves a worktree selector and removes it. It returns the
// data the CLI reports and avoids Timber command types in its signature.
func (runtime Runtime) RemoveWorktree(input io.Reader, raw string, force bool) (string, string, string, bool, error) {
	selection := repoSelection{runtime: runtime}
	_, repository, worktree, err := selection.resolveQualifiedWorktree(input, raw)
	if err != nil {
		return "", "", "", false, err
	}
	result, err := runtime.removeResolvedWorktree(repository, worktree, force)
	if err != nil {
		return "", "", "", false, err
	}
	return result.worktree.Name, result.worktree.shortCommitHash(), result.worktree.Path, result.removingCurrentDirectory, nil
}

// RenameRepo renames a registered repository and its managed worktrees. It
// returns the source and destination names for CLI reporting.
func (runtime Runtime) RenameRepo(oldName, newName string) (string, string, error) {
	plan, err := runtime.buildRepositoryRenamePlan(oldName, newName)
	if err != nil {
		return "", "", err
	}

	if err := runtime.reportRenamedCurrentPath(plan.currentTargetDir); err != nil {
		return "", "", err
	}
	repair := func(barePath string, worktreePaths []string) error {
		return repairWorktrees(runtime, barePath, worktreePaths)
	}
	if err := plan.apply(os.Rename, repair); err != nil {
		return "", "", err
	}

	return plan.sourceRepo.Name, plan.destinationRepo.Name, nil
}

func (x Runtime) ReportCreatedWorktreePath(output io.Writer, worktreePath string) error {
	if pathFile := x.CreatePathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
			return fmt.Errorf("write created worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(output, worktreePath)
	return err
}

func (x Runtime) ReportSwitchWorktreePath(output io.Writer, worktreePath string) error {
	if pathFile := x.SwitchPathFile; pathFile != "" {
		if err := x.writePathFile(pathFile, worktreePath); err != nil {
			return fmt.Errorf("write switch worktree path file: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(output, worktreePath)
	return err
}

// ResolveGitSelector interprets a candidate as a <worktree>, <worktree>@<repo>,
// or @<repo> selector. A candidate containing @ always selects a target and
// returns its errors. A bare worktree name selects the managed worktree, while
// a repo-only @<repo> selects the bare repository. A bare name selects a
// worktree only when it resolves to an existing managed worktree; otherwise
// selected is false and the caller can treat the candidate as a Git subcommand so
// commands such as `status` keep working. The returned gitDir is set for the repo-only
// case; it is empty for worktrees so the caller resolves it from the
// worktree directory.
func (runtime Runtime) ResolveGitSelector(input io.Reader, candidate string) (string, string, bool, error) {
	x := repoSelection{runtime: runtime}
	if strings.Contains(candidate, "@") {
		qualified, err := x.runtime.parseQualifiedName(candidate)
		if err != nil {
			return "", "", false, err
		}
		if qualified.Name == "" {
			repo, err := x.runtime.registeredRepoByName(qualified.Repo)
			if err != nil {
				return "", "", false, err
			}
			return repo.BarePath, repo.BarePath, true, nil
		}
		_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
		if err != nil {
			return "", "", false, err
		}
		return worktree.Path, "", true, nil
	}

	repoName, err := x.runtime.inferUniqueRepoForWorktree(candidate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	x.RepoName = repoName
	_, _, worktree, err := x.resolveQualifiedWorktree(input, candidate)
	if err != nil {
		return "", "", false, err
	}
	return worktree.Path, "", true, nil
}

// ResolveHerdrWorktree resolves a worktree selector to its repository,
// name, and path for Herdr operations.
func (x Runtime) ResolveHerdrWorktree(input io.Reader, raw string) (repo, name, path string, err error) {
	selection := repoSelection{runtime: x}
	_, _, worktree, err := selection.resolveQualifiedWorktree(input, raw)
	if err != nil {
		return "", "", "", err
	}
	return worktree.Repo, worktree.Name, worktree.Path, nil
}

// ResolveSwitchWorktree resolves a worktree selector to its name and path.
// alreadyIn reports whether the current directory is already the target.
func (runtime Runtime) ResolveSwitchWorktree(input io.Reader, raw, repoName string) (name, path string, alreadyIn bool, err error) {
	qualified, err := runtime.parseQualifiedName(raw)
	if err != nil {
		return "", "", false, err
	}
	if qualified.Repo != "" {
		repoName = qualified.Repo
	}

	name = qualified.Name
	if name == "" {
		return "", "", false, fmt.Errorf("worktree name is required")
	}
	if repoName == "" {
		repoName, err = runtime.inferUniqueRepoForWorktree(name)
		if err != nil {
			return "", "", false, err
		}
	}

	path, err = switchWorktreePath(runtime, repoName, name)
	if err != nil {
		return "", "", false, err
	}

	same, err := samePath(runtime.CurrentDirectory, path)
	if err != nil {
		return "", "", false, err
	}
	return name, path, same, nil
}

func (x Runtime) ResolveTodoGitDir(input io.Reader, args []string) (string, error) {
	if len(args) == 0 {
		gitDir, err := gitOutput(x, x.CurrentDirectory, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
		}
		bare, err := gitOutput(x, x.CurrentDirectory, "rev-parse", "--is-bare-repository")
		if err != nil {
			return "", err
		}
		if bare.stdout == "true" {
			return "", fmt.Errorf("not inside a worktree: pass a worktree name or run inside a worktree")
		}
		return filepath.Clean(gitDir.stdout), nil
	}
	selection := repoSelection{runtime: x}
	_, _, worktree, err := selection.resolveQualifiedWorktree(input, args[0])
	if err != nil {
		return "", err
	}
	repository, err := openRepository(x, worktree.Path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(repository.GitDir), nil
}

func (x Runtime) ResolveTodoRepoGitDir(input io.Reader, args []string) (string, error) {
	gitDir, err := x.ResolveTodoGitDir(input, args)
	if err != nil {
		return "", err
	}
	result, err := gitOutput(x, gitDir, "--git-dir", gitDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("resolve common git dir: %w", err)
	}
	return filepath.Clean(result.stdout), nil
}

// RunTUI runs the interactive worktree wizard using the supplied prompter.
// A cancelled wizard returns nil without creating or opening anything.
func (runtime Runtime) RunTUI(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool, prompter WizardPrompter, reporters CreateReporters) error {
	if prompter == nil {
		return errors.New("tui requires a wizard prompter")
	}
	repos, err := runtime.listRegisteredReposForWizard()
	if err != nil {
		return err
	}

	worktrees, err := runtime.listWorktreesForWizard(repos)
	if err != nil {
		return err
	}

	repoRefs := make([]RepoRef, 0, len(repos))
	for _, repo := range repos {
		repoRefs = append(repoRefs, repo.ref())
	}
	worktreeRefs := make([]WorktreeRef, 0, len(worktrees))
	for _, worktree := range worktrees {
		worktreeRefs = append(worktreeRefs, worktree.ref())
	}
	selection, err := prompter.Prompt(input, output, repoRefs, worktreeRefs, !noTitle)
	if err != nil || isSame(selection.Cancelled, true) {
		return err
	}

	if selection.Action == WizardActionOpen {
		return runtime.openSelectedWorktree(ctx, output, selection, reporters)
	}
	return runtime.createSelectedWorktree(ctx, input, output, stdout, selection, herdr, noHerdr, reporters)
}

// SaveConfig saves the persistent Timber settings.
func (x Runtime) SaveConfig(settings coreconfig.Settings) error {
	return writeConfig(x.configPath(), settings)
}

func (x Runtime) absolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if x.CurrentDirectory == "" {
		return "", fmt.Errorf("current directory is not set")
	}
	return filepath.Abs(filepath.Join(x.CurrentDirectory, path))
}

func (x Runtime) autoInstallHKHooks(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter, reportWarning func(io.Writer, SetupKind, error) error) {
	reportSetupWarning(output, reportWarning, SetupHKInstall, installHKHooks(x, ctx, input, output, repository, upstream, worktreePath, prompter))
}

func (x Runtime) autoTrustMise(ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter, reportWarning func(io.Writer, SetupKind, error) error) {
	reportSetupWarning(output, reportWarning, SetupMiseTrust, trustMiseConfig(x, ctx, input, output, repository, upstream, worktreePath, prompter))
}

// bareRepoPath returns <worktree-root>/<repo-name>/.git.
// The Git directory is named .git so a scanner that looks for a child
// .git directory can see the repository placeholder. Git still reports
// the parent directory as the bare worktree path.
func (x Runtime) bareRepoPath(repoName string) string {
	return filepath.Join(x.worktreeRoot(), filepath.FromSlash(repoName), bareRepoSuffix)
}

func (x Runtime) buildImportPlan(sourcePath string, requestedName string) (importPlan, error) {
	sourceRepository, err := openImportSource(x, sourcePath)
	if err != nil {
		return importPlan{}, err
	}
	return importPlanForRepository(x, sourceRepository, requestedName)
}

func (x Runtime) buildRepositoryRenamePlan(oldName string, requestedNewName string) (repositoryRenamePlan, error) {
	sourceRepo, err := x.registeredRepoByName(oldName)
	if err != nil {
		return repositoryRenamePlan{}, err
	}

	newName := normalizeRepoName(requestedNewName)
	if err := validateRepoName(newName); err != nil {
		return repositoryRenamePlan{}, err
	}
	if sourceRepo.Name == newName {
		return repositoryRenamePlan{}, fmt.Errorf("repository is already named %q", newName)
	}

	plan := repositoryRenamePlan{
		runtime:    x,
		sourceRepo: sourceRepo,
		destinationRepo: registeredRepo{
			Name:     newName,
			BarePath: x.bareRepoPath(newName),
		},
	}
	if err := plan.validateDestinationRepo(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.collectWorktrees(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.validateWorktreeDestinations(); err != nil {
		return repositoryRenamePlan{}, err
	}
	if err := plan.findCurrentTargetDirectory(); err != nil {
		return repositoryRenamePlan{}, err
	}
	return plan, nil
}

func (x Runtime) collectManagedWorktrees(repos []registeredRepo) ([]managedWorktree, error) {
	return x.collectWorktrees(repos, x.enrichManagedWorktree)
}

func (x Runtime) collectWorktrees(repos []registeredRepo, enrich worktreeEnricher) ([]managedWorktree, error) {
	worktrees := make([]managedWorktree, 0)
	for _, repo := range repos {
		repository, err := openBareRepository(x, repo.BarePath)
		if err != nil {
			return nil, err
		}

		repoWorktrees, err := x.managedWorktreesFromRepository(repository, repo.Name)
		if err != nil {
			return nil, err
		}

		for _, worktree := range repoWorktrees {
			enrichedWorktree, err := enrich(repository, worktree)
			if err != nil {
				return nil, err
			}
			worktrees = append(worktrees, enrichedWorktree)
		}
	}

	slices.SortFunc(worktrees, compareManagedWorktrees)
	return worktrees, nil
}

func (x Runtime) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if name == "herdr" && !isEmpty(x.HerdrExecutable) {
		name = x.HerdrExecutable
	}
	if name == "gh" && !isEmpty(x.GhExecutable) {
		name = x.GhExecutable
	}

	command := exec.CommandContext(ctx, name, args...)
	if x.Environment != nil {
		command.Env = slices.Clone(x.Environment)
	}
	return command
}

func (x Runtime) configPath() string {
	return filepath.Join(x.xdgConfigHome(), "timber", "config.toml")
}

func (x Runtime) createHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	parentID, err := x.ensureHerdrParentWorkspace(ctx, worktree.Repo)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(
		ctx,
		"worktree", "open", "--workspace", parentID, "--path", absolutePath,
		"--label", worktree.Name, "--no-focus",
	)
	if err != nil {
		return herdrSpace{}, err
	}

	return parseHerdrSpace(x, output, absolutePath, qualifiedWorktreeName(worktree.Name, worktree.Repo))
}

func (runtime Runtime) createSelectedWorktree(
	ctx context.Context,
	input io.Reader,
	output, stdout io.Writer,
	selection WizardSelection,
	herdr, noHerdr bool,
	reporters CreateReporters,
) error {
	worktreePath, _, err := runtime.CreateManagedWorktree(
		ctx,
		input,
		output,
		selection.RepoName,
		selection.WorktreeName,
		"",
		false,
		"",
		herdr,
		noHerdr,
		reporters,
	)
	if err != nil {
		return err
	}
	return runtime.ReportCreatedWorktreePath(stdout, worktreePath)
}

func (x Runtime) currentHerdrSpace(ctx context.Context, worktree managedWorktree) (herdrSpace, error) {
	absolutePath, err := x.herdrWorktreePath(worktree)
	if err != nil {
		return herdrSpace{}, err
	}

	output, err := x.runHerdr(ctx, "pane", "current", "--current")
	if err != nil {
		return herdrSpace{}, err
	}
	return parseCurrentHerdrSpace(x, output, absolutePath, qualifiedWorktreeName(worktree.Name, worktree.Repo))
}

func (x Runtime) defineCurrentHerdrSpace(ctx context.Context, worktree managedWorktree) error {
	space, err := x.currentHerdrSpace(ctx, worktree)
	if err != nil {
		return err
	}
	if err := space.configure(ctx); err != nil {
		return err
	}
	return space.focus(ctx)
}

// displayHomePath replaces a leading home directory with "~" for display.
func (x Runtime) displayHomePath(path string) string {
	home := x.HomeDirectory
	if home == "" {
		return path
	}

	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	prefix := cleanHome + string(filepath.Separator)
	if relative, ok := strings.CutPrefix(cleanPath, prefix); ok {
		return "~" + string(filepath.Separator) + relative
	}
	return path
}

// enableWorktreeConfig turns on per-worktree configuration for a repository.
//
// Git stops special-casing core.bare once this extension is on, so a bare
// repository must first record the bare marker in its main worktree
// configuration and drop it from the shared configuration. Git ignores
// config.worktree while the extension is off, so the two writes leave the
// repository usable at every step.
func (runtime Runtime) enableWorktreeConfig(commonDir string) error {
	enabled, err := gitBooleanSetting(runtime, commonDir, "extensions.worktreeConfig")
	if err != nil {
		return err
	}
	if enabled {
		return nil
	}

	bare, err := gitBooleanSetting(runtime, commonDir, "core.bare")
	if err != nil {
		return err
	}
	if bare {
		mainWorktreeConfig := filepath.Join(commonDir, "config.worktree")
		if _, err := gitOutput(runtime, commonDir, "config", "--file", mainWorktreeConfig, "core.bare", "true"); err != nil {
			return fmt.Errorf("record bare repository marker: %w", err)
		}
		if _, err := gitOutput(runtime, commonDir, "config", "--unset", "core.bare"); err != nil {
			return fmt.Errorf("remove shared bare repository marker: %w", err)
		}
	}

	if _, err := gitOutput(runtime, commonDir, "config", "extensions.worktreeConfig", "true"); err != nil {
		return fmt.Errorf("enable per-worktree configuration: %w", err)
	}
	return nil
}

// enrichListedWorktreesWithPullRequests attaches one gh pull request lookup
// per repository. It runs after collection so each repo needs a single call.
func (x Runtime) enrichListedWorktreesWithPullRequests(ctx context.Context, worktrees []managedWorktree) error {
	byRepo := make(map[string][]int)
	for index, worktree := range worktrees {
		byRepo[worktree.Repo] = append(byRepo[worktree.Repo], index)
	}

	repoNames := make([]string, 0, len(byRepo))
	for repoName := range byRepo {
		repoNames = append(repoNames, repoName)
	}
	slices.Sort(repoNames)

	for _, repoName := range repoNames {
		repo, err := x.registeredRepoByName(repoName)
		if err != nil {
			return err
		}
		output, err := x.runGh(ctx, repo.BarePath, "pr", "list",
			"--json", "number,headRefName,state,statusCheckRollup",
			"--limit", strconv.Itoa(pullRequestListLimit),
		)
		if err != nil {
			return err
		}
		pullRequests, err := parsePullRequestList(output)
		if err != nil {
			return err
		}
		for _, index := range byRepo[repoName] {
			worktrees[index].PullRequest = pullRequests[worktrees[index].branchName()]
		}
	}
	return nil
}

func (x Runtime) enrichListedWorktreesWithReviews(ctx context.Context, worktrees []managedWorktree) {
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(reviewLookupConcurrency, len(worktrees)) {
		workers.Go(func() {
			for index := range jobs {
				if ctx.Err() != nil {
					continue
				}
				worktrees[index].Review = x.worktreeReviewProgress(ctx, worktrees[index].Path)
			}
		})
	}
	for index := range worktrees {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
}

func (x Runtime) enrichManagedWorktree(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	worktreeRepository, err := openRepository(x, worktree.Path)
	if err != nil {
		return managedWorktree{}, err
	}

	clean, err := worktreeRepository.isClean()
	if err != nil {
		return managedWorktree{}, err
	}

	status, err := worktreeRepository.status()
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.Status = status
	worktree.Clean = clean

	upstreamRef, err := repository.upstreamReference(worktree.branchName())
	if err != nil {
		// Manual branches need not track an upstream. They remain visible
		// to the prune picker, but are never eligible for automatic pruning.
		_, configured, configErr := repository.gitConfigValue("branch." + worktree.branchName() + ".merge")
		if configErr != nil {
			return managedWorktree{}, configErr
		}
		if !configured {
			return worktree, nil
		}
		return managedWorktree{}, err
	}

	merged, err := repository.branchMergedToUpstream(worktree.branchName(), worktree.BranchReference, upstreamRef)
	if err != nil {
		return managedWorktree{}, err
	}

	worktree.UpstreamRef = upstreamRef
	worktree.Merged = merged

	return worktree, nil
}

func (x Runtime) enrichWorktreeForList(repository *Repository, worktree managedWorktree) (managedWorktree, error) {
	result, err := gitOutput(x, worktree.Path, "status", "--porcelain=v2", "--branch")
	if err != nil {
		worktree.ListError = true
		return worktree, nil
	}

	status, clean, err := parsePorcelainStatus(result.stdout)
	if err != nil {
		worktree.ListError = true
		return worktree, nil
	}
	worktree.ListStatus = status
	worktree.Clean = clean
	worktree.Todo = x.todoProgressForWorktree(worktree)
	// Record the repo default so Status can hide it and only call out
	// worktrees tracking something else. Unknown defaults show upstream.
	if defaultBranch, err := repository.remoteHeadBranch(); err == nil {
		worktree.DefaultUpstream = defaultBranch
	}

	// A branch without a resolvable upstream is not merged anywhere timber
	// tracks; list stays read-only and reports it as unmerged.
	if upstreamRef, err := repository.upstreamReference(worktree.branchName()); err == nil {
		merged, err := repository.branchMergedToUpstream(worktree.branchName(), worktree.BranchReference, upstreamRef)
		if err != nil {
			return managedWorktree{}, err
		}
		worktree.Merged = merged
	}
	return worktree, nil
}

// ensureHerdrParentWorkspace returns the ID of the Herdr workspace rooted at
// the repository's bare directory, creating it when no such workspace exists.
// A parent left behind by an earlier workflow keeps its label unless it is
// exactly the bare directory name; only that artifact is renamed to the
// repository name.
func (x Runtime) ensureHerdrParentWorkspace(ctx context.Context, repoName string) (string, error) {
	barePath := filepath.Clean(x.bareRepoPath(repoName))

	output, err := x.runHerdr(ctx, "worktree", "list", "--cwd", barePath)
	if err != nil {
		return "", err
	}
	parentID, err := parseHerdrWorktreeListParent(output)
	if err != nil {
		return "", err
	}
	if parentID == "" {
		output, err := x.runHerdr(
			ctx,
			"workspace", "create", "--cwd", barePath,
			"--label", repoName, "--no-focus",
		)
		if err != nil {
			return "", err
		}
		return x.setupHerdrParentDashboard(ctx, output, repoName, barePath)
	}

	output, err = x.runHerdr(ctx, "workspace", "get", parentID)
	if err != nil {
		return "", err
	}
	label, err := parseHerdrWorkspaceLabel(output, parentID)
	if err != nil {
		return "", err
	}
	if label == repoName+bareRepoSuffix {
		if _, err := x.runHerdr(ctx, "workspace", "rename", parentID, repoName); err != nil {
			return "", err
		}
	}
	return parentID, nil
}

// environmentValue returns the last value set for name in the runtime
// environment, matching how the process would resolve it.
func (x Runtime) environmentValue(name string) string {
	prefix := name + "="
	for _, entry := range slices.Backward(x.Environment) {
		if value, found := strings.CutPrefix(entry, prefix); found {
			return value
		}
	}
	return ""
}

func (x Runtime) fetchGitHubIdentity(ctx context.Context) (githubIdentity, error) {
	if _, err := x.runGh(ctx, x.CurrentDirectory, "auth", "status", "--hostname", "github.com"); err != nil {
		return githubIdentity{}, err
	}
	userJSON, err := x.runGh(ctx, x.CurrentDirectory, "api", "--hostname", "github.com", "user")
	if err != nil {
		return githubIdentity{}, err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(userJSON, &user); err != nil {
		return githubIdentity{}, fmt.Errorf("decode GitHub user: %w", err)
	}
	orgsJSON, err := x.runGh(ctx, x.CurrentDirectory, "api", "--hostname", "github.com", "--paginate", "--slurp", "user/orgs")
	if err != nil {
		return githubIdentity{}, err
	}
	var pages [][]struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(orgsJSON, &pages); err != nil {
		return githubIdentity{}, fmt.Errorf("decode GitHub organizations: %w", err)
	}
	identity := githubIdentity{Username: user.Login, FetchedAt: time.Now().UTC()}
	for _, page := range pages {
		for _, organization := range page {
			identity.Organizations = append(identity.Organizations, organization.Login)
		}
	}
	if !identity.valid() {
		return githubIdentity{}, errors.New("GitHub returned an invalid identity")
	}
	return identity, nil
}

// ghActiveUser returns the GitHub CLI user for github.com. The keyring slot
// for a token is named after that user.
func (x Runtime) ghActiveUser() (string, error) {
	path := filepath.Join(x.ghConfigDirectory(), ghHostsFileName)
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	// The file can also hold a token. Timber never reads it: a secret does not
	// belong in a configuration file.
	var hosts map[string]struct {
		User string `yaml:"user"`
	}
	if err := yaml.Unmarshal(contents, &hosts); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return hosts[githubHost].User, nil
}

// ghAuthOK reports whether gh can talk to GitHub. The dashboard probes once
// at setup; a missing CLI or login only drops the PR column, never the space.
func (x Runtime) ghAuthOK(ctx context.Context) bool {
	command := x.command(ctx, "gh", "auth", "status")
	return command.Run() == nil
}

// ghConfigDirectory returns the directory the GitHub CLI reads its
// configuration from.
func (x Runtime) ghConfigDirectory() string {
	if x.GhConfigDirectory != "" {
		return x.GhConfigDirectory
	}
	if directory := x.environmentValue("GH_CONFIG_DIR"); directory != "" {
		return directory
	}
	return filepath.Join(x.ConfigHome, "gh")
}

func (x Runtime) githubIdentity(ctx context.Context) (githubIdentity, error) {
	path := filepath.Join(x.xdgDataHome(), "timber", "github-identity.json")
	contents, cacheErr := os.ReadFile(path)
	if cacheErr == nil {
		var identity githubIdentity
		cacheErr = json.Unmarshal(contents, &identity)
		if cacheErr == nil {
			if identity.valid() {
				return identity, nil
			}
			cacheErr = errors.New("expired or invalid identity")
		}
	}

	// Bound the optional network lookup so worktree creation cannot wait forever.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	identity, err := x.fetchGitHubIdentity(ctx)
	if err != nil {
		return githubIdentity{}, fmt.Errorf("GitHub identity cache is unavailable (%v); refresh failed: %w", cacheErr, err)
	}
	contents, err = json.Marshal(identity)
	if err != nil {
		return githubIdentity{}, err
	}
	if err := writePrivateFile(path, contents); err != nil {
		return githubIdentity{}, fmt.Errorf("save GitHub identity cache: %w", err)
	}
	return identity, nil
}

// githubOwnedWorktree reports whether the selected upstream resolves to a
// GitHub repository owned by the cached user or one of their organizations.
func (x Runtime) githubOwnedWorktree(ctx context.Context, repository *Repository, upstream string) (githubIdentity, bool, error) {
	remoteURL, err := repository.upstreamRemoteURL(upstream)
	if err != nil {
		return githubIdentity{}, false, err
	}
	owner := githubRemoteOwner(remoteURL)
	if owner == "" {
		return githubIdentity{}, false, nil
	}
	identity, err := x.githubIdentity(ctx)
	if err != nil {
		return githubIdentity{}, false, err
	}
	if !identity.owns(owner) {
		return githubIdentity{}, false, nil
	}
	return identity, true, nil
}

// githubToken returns an authentication token for the GitHub API.
//
// Sources are tried in order: GH_TOKEN, GITHUB_TOKEN, then the credential the
// GitHub CLI stored in the system keyring. Timber reads no token from a file:
// no configuration file holds a secret, and no error message repeats a token
// value.
func (x Runtime) githubToken() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := x.environmentValue(name); token != "" {
			return token, nil
		}
	}

	user, err := x.ghActiveUser()
	if err != nil {
		return "", err
	}
	// The keyring can hold an entry for the active user and an older entry
	// without one, so both slots are read.
	slots := []string{""}
	if user != "" {
		slots = []string{user, ""}
	}
	for _, slot := range slots {
		token, found, err := x.keyringToken(slot)
		if err != nil {
			return "", err
		}
		if found {
			return token, nil
		}
	}

	return "", fmt.Errorf("no GitHub token for %s; set GH_TOKEN or run: gh auth login", githubHost)
}

func (x Runtime) herdrConfigFilePath() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "config.toml")
}

func (x Runtime) herdrPluginInstallDirectory() string {
	return filepath.Join(x.xdgConfigHome(), "herdr", "plugins", herdrPluginDirectoryName)
}

func (x Runtime) herdrWorktreePath(worktree managedWorktree) (string, error) {
	absolutePath, err := x.absolutePath(worktree.Path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path for herdr: %w", err)
	}
	return absolutePath, nil
}

func (x Runtime) inferUniqueRepoForWorktree(worktreeName string) (string, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return "", err
	}

	var matches []string
	for _, repo := range repos {
		worktrees, err := x.worktreesForRepo(repo)
		if err != nil {
			return "", err
		}
		if _, err := managedWorktreeByName(worktrees, worktreeName); err == nil {
			matches = append(matches, repo.Name)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("worktree %s not found", worktreeName)
	default:
		slices.Sort(matches)
		return "", fmt.Errorf(
			"worktree %q exists in multiple repositories; qualify as <worktree>@<repo> (%s)",
			worktreeName,
			strings.Join(matches, ", "),
		)
	}
}

// keyringToken reads a gh keyring entry. An unavailable keyring, an empty
// slot, and a timeout all just mean no stored token.
func (x Runtime) keyringToken(user string) (string, bool, error) {
	lookup := x.GitHubKeyringLookup
	if lookup == nil {
		lookup = lookupGitHubKeyring
	}
	token, err := lookup(ghKeyringServiceName, user)
	if err != nil {
		var timeout *keyringTimeoutError
		switch {
		case errors.Is(err, keyring.ErrNotFound), errors.As(err, &timeout):
			return "", false, nil
		default:
			return "", false, fmt.Errorf("read GitHub keyring entry: %w", err)
		}
	}
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

func (x Runtime) legacyReposDirectory() string {
	return filepath.Join(x.xdgDataHome(), legacyReposDirName)
}

func (x Runtime) listLegacyRepos() ([]registeredRepo, error) {
	directory := x.legacyReposDirectory()
	if _, err := os.Stat(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make([]registeredRepo, 0), nil
		}
		return nil, fmt.Errorf("read legacy repos directory %q: %w", directory, err)
	}

	walk := legacyRepoWalk{runtime: x, directory: directory}
	walkErr := filepath.WalkDir(directory, walk.visit)
	if walkErr != nil {
		return nil, fmt.Errorf("read legacy repos directory %q: %w", directory, walkErr)
	}
	slices.SortFunc(walk.repos, func(left, right registeredRepo) int {
		return strings.Compare(left.Name, right.Name)
	})
	return walk.repos, nil
}

func (x Runtime) listRegisteredRepos() ([]registeredRepo, error) {
	root := x.worktreeRoot()
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make([]registeredRepo, 0), nil
		}
		return nil, fmt.Errorf("read worktree root %q: %w", root, err)
	}

	repos := make([]registeredRepo, 0)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		repo, found, skipChildren := x.registeredRepoAt(root, path)
		if found {
			repos = append(repos, repo)
		}
		if skipChildren {
			return filepath.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("read worktree root %q: %w", root, walkErr)
	}

	slices.SortFunc(repos, func(left, right registeredRepo) int {
		return strings.Compare(left.Name, right.Name)
	})
	return repos, nil
}

func (x Runtime) listRegisteredReposForWizard() ([]registeredRepo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, errors.New("no registered repositories; run timber repo add first")
	}
	return repos, nil
}

func (x Runtime) listWorktreesForWizard(repos []registeredRepo) ([]managedWorktree, error) {
	return x.collectWorktrees(repos, func(_ *Repository, worktree managedWorktree) (managedWorktree, error) {
		return worktree, nil
	})
}

func (x Runtime) loadConfig() (config, error) {
	path := x.configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return config{}, fmt.Errorf("create config directory: %w", err)
	}
	// Do not overwrite settings if another process creates the file first.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		encodeErr := toml.NewEncoder(file).Encode(config{})
		if err := errors.Join(encodeErr, file.Close()); err != nil {
			return config{}, fmt.Errorf("create config: %w", err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return config{}, fmt.Errorf("create config: %w", err)
	}
	var settings config
	metadata, err := toml.DecodeFile(path, &settings)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return config{}, fmt.Errorf("unknown config setting: %s", keys[0])
	}
	return settings, nil
}

// managedWorktreePath returns
// <worktree-root>/<repo-name>/<worktree-name>/<repo-short-name>.
func (x Runtime) managedWorktreePath(repoName string, worktreeName string) string {
	return filepath.Join(x.worktreeRoot(), filepath.FromSlash(repoName), worktreeName, repoShortName(repoName))
}

func (x Runtime) managedWorktreesFromRepository(repository *Repository, repoName string) ([]managedWorktree, error) {
	porcelainWorktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return nil, err
	}

	currentDirectory := x.CurrentDirectory

	managedWorktrees := make([]managedWorktree, 0)
	for _, porcelainWorktree := range porcelainWorktrees {
		branchName := porcelainWorktree.branchName()
		if branchName == "" {
			continue
		}

		name, err := x.recordedWorktreeName(repoName, porcelainWorktree.Path, branchName)
		if err != nil {
			return nil, err
		}

		managedWorktrees = append(managedWorktrees, managedWorktree{
			Repo:            repoName,
			Name:            name,
			Path:            porcelainWorktree.Path,
			DisplayPath:     currentRelativePath(currentDirectory, porcelainWorktree.Path),
			CommitHash:      porcelainWorktree.CommitHash,
			BranchReference: referenceName(porcelainWorktree.BranchRef),
		})
	}

	assignManualWorktreeNames(managedWorktrees)
	slices.SortFunc(managedWorktrees, compareManagedWorktrees)

	return managedWorktrees, nil
}

func (x Runtime) moveLegacyRepo(repo registeredRepo) (bool, error) {
	destination := x.bareRepoPath(repo.Name)
	same, err := samePath(repo.BarePath, destination)
	if err != nil {
		return false, err
	}
	if same {
		return false, nil
	}
	if _, err := os.Lstat(destination); err == nil {
		return false, fmt.Errorf("repository %q already exists at %s", repo.Name, destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect repository path %q: %w", destination, err)
	}
	if err := ensureDirectory(filepath.Dir(destination)); err != nil {
		return false, err
	}
	if err := os.Rename(repo.BarePath, destination); err != nil {
		return false, fmt.Errorf("move bare repository %q to %q: %w", repo.BarePath, destination, err)
	}
	if err := removeEmptyParents(repo.BarePath, x.legacyReposDirectory()); err != nil {
		return false, err
	}
	return true, nil
}

// movedInternalWorktree recognizes a checkout that moved with a legacy bare
// repository. Both old links must agree before Git can repair either one.
func (x Runtime) movedInternalWorktree(repo registeredRepo, oldPath string) (string, error) {
	path, ok, err := relocatedWorktreePath(x, repo, oldPath)
	if err != nil || !isTrue(ok) {
		return "", err
	}
	return matchingMovedGitDir(x, repo, oldPath, path)
}

// newGitHubClient builds the API client. The token is passed straight to the
// client and never stored on the runtime or reported.
func (runtime Runtime) newGitHubClient(token string) (*github.Client, error) {
	options := []github.ClientOptionsFunc{
		github.WithAuthToken(token),
		github.WithUserAgent("timber"),
	}
	if runtime.GitHubAPIBaseURL != "" {
		options = append(options, github.WithURLs(new(runtime.GitHubAPIBaseURL), new(runtime.GitHubAPIBaseURL)))
	}
	client, err := github.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create GitHub client: %w", err)
	}
	return client, nil
}

func (x Runtime) openHerdrSpace(ctx context.Context, worktree managedWorktree) (returnErr error) {
	space, returnErr := x.createHerdrSpace(ctx, worktree)
	if space.workspaceID == "" {
		return returnErr
	}
	if space.alreadyOpen {
		return space.focusWorkspace(ctx)
	}

	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, space.close(context.WithoutCancel(ctx)))
		}
	}()

	if returnErr != nil {
		return returnErr
	}
	if returnErr = space.configure(ctx); returnErr != nil {
		return returnErr
	}
	return space.focus(ctx)
}

func (x Runtime) openRegisteredRepository(name string) (*Repository, registeredRepo, error) {
	repo, err := x.registeredRepoByName(name)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return nil, registeredRepo{}, err
	}
	return repository, repo, nil
}

func (runtime Runtime) openSelectedWorktree(
	ctx context.Context,
	output io.Writer,
	selection WizardSelection,
	reporters CreateReporters,
) error {
	if isNilFunc(reporters.HerdrOpened) {
		return errors.New("tui requires a Herdr opening reporter")
	}
	repository, _, err := runtime.openRegisteredRepository(selection.RepoName)
	if err != nil {
		return err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, selection.RepoName)
	if err != nil {
		return err
	}
	worktree, err := managedWorktreeByName(worktrees, selection.WorktreeName)
	if err != nil {
		return err
	}
	if err := runtime.openHerdrSpace(ctx, worktree); err != nil {
		return err
	}
	return reporters.HerdrOpened(output, worktree.Name)
}

func (x Runtime) parseQualifiedName(raw string) (qualifiedName, error) {
	name, repo, found := strings.CutLast(raw, "@")
	if !found {
		return qualifiedName{Name: raw}, nil
	}
	if repo == "" {
		return qualifiedName{}, fmt.Errorf("missing repository name after @")
	}
	if _, err := x.registeredRepoByName(repo); err != nil {
		return qualifiedName{}, err
	}
	return qualifiedName{Name: name, Repo: repo}, nil
}

func (x Runtime) parseRepoOnlyArg(raw string) (string, error) {
	qualified, err := x.parseQualifiedName(raw)
	if err != nil {
		return "", err
	}
	if qualified.Name != "" || isEmpty(qualified.Repo) {
		return "", fmt.Errorf("expected @<repo>, got %q", raw)
	}
	return qualified.Repo, nil
}

// pullRequestTarget resolves the worktree selector and reads what the pull
// request needs from the repository. It runs before any GitHub request so a
// local problem never needs a token.
func (runtime Runtime) pullRequestTarget(input io.Reader, draft PullRequestDraft) (pullRequestTarget, error) {
	selection := repoSelection{runtime: runtime}
	_, repository, worktree, err := selection.resolveQualifiedWorktree(input, draft.Raw)
	if err != nil {
		return pullRequestTarget{}, err
	}

	head := worktree.branchName()
	if head == "" {
		return pullRequestTarget{}, fmt.Errorf("worktree %q has a detached HEAD; a pull request needs a branch", worktree.Name)
	}

	remote, owner, repo, err := pullRequestRemoteParts(repository, worktree, head)
	if err != nil {
		return pullRequestTarget{}, err
	}

	base := draft.Base
	if base == "" {
		base, err = repository.pullRequestBase(head, remote)
		if err != nil {
			return pullRequestTarget{}, err
		}
	}

	headSHA, err := localHeadCommit(runtime, worktree.Path)
	if err != nil {
		return pullRequestTarget{}, err
	}

	return pullRequestTarget{remote: remote, owner: owner, repo: repo, head: head, base: base, headSHA: headSHA}, nil
}

// Standard-layout worktrees predate explicit name records. An empty name
// identifies a manual checkout whose name must be derived from its path.
func (x Runtime) recordedWorktreeName(repoName, path, branch string) (string, error) {
	same, err := samePath(path, x.managedWorktreePath(repoName, branch))
	if err != nil {
		return "", err
	}
	if same {
		return branch, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	gitDir, err := x.worktreeGitDir(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(gitDir, worktreeNameFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read worktree name: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

// registeredRepoAt reports a bare repository whose Git directory is path/.git.
// skipChildren is true when path is a repository placeholder or a linked
// checkout, so the walk does not enter object databases or worktree files.
func (x Runtime) registeredRepoAt(root string, path string) (registeredRepo, bool, bool) {
	gitDir := filepath.Join(path, bareRepoSuffix)
	info, err := os.Lstat(gitDir)
	if err != nil {
		return registeredRepo{}, false, false
	}
	// The worktree root can itself contain a .git entry. Do not treat that
	// as a reason to skip every repository under it.
	if path == root {
		return registeredRepo{}, false, false
	}
	if !info.IsDir() {
		return registeredRepo{}, false, true
	}
	bare, bareErr := pathIsBareRepository(x, gitDir)
	if !bare || !isSame(bareErr, nil) {
		return registeredRepo{}, false, true
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return registeredRepo{}, false, true
	}
	repoName := filepath.ToSlash(relative)
	if err := validateRepoName(repoName); err != nil {
		return registeredRepo{}, false, true
	}
	return registeredRepo{Name: repoName, BarePath: gitDir}, true, true
}

func (x Runtime) registeredRepoByName(name string) (registeredRepo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, err
	}
	index := slices.IndexFunc(repos, func(repo registeredRepo) bool {
		return repo.Name == name
	})
	if index >= 0 {
		return repos[index], nil
	}
	return registeredRepo{}, fmt.Errorf("unknown repository %q", name)
}

// removeEmptyBareParents prunes newly-empty grouping directories left
// behind under the worktree root (e.g. worktrees/nnutter/ after moving
// worktrees/nnutter/timber/.git away). It never removes the root itself.
func (x Runtime) removeEmptyBareParents(path string) error {
	return removeEmptyParents(path, x.worktreeRoot())
}

func (x Runtime) removeEmptySourceParents(path string) error {
	return removeEmptyParents(path, x.HomeDirectory)
}

func (runtime Runtime) removeResolvedWorktree(repository *Repository, worktree managedWorktree, force bool) (worktreeRemovalResult, error) {
	name := worktree.Name
	standardLayout, err := samePath(worktree.Path, runtime.managedWorktreePath(worktree.Repo, worktree.branchName()))
	if err != nil {
		return worktreeRemovalResult{}, err
	}

	worktree, err = refuseUnsafeRemoval(runtime, repository, worktree, name, force)
	if err != nil {
		return worktreeRemovalResult{}, err
	}

	currentDirectory := runtime.CurrentDirectory
	removingCurrentDirectory := currentDirectory != "" && pathIsWithin(worktree.Path, currentDirectory)

	removeArguments := []string{"worktree", "remove"}
	if force {
		removeArguments = append(removeArguments, "--force")
	}
	removeArguments = append(removeArguments, worktree.Path)
	if _, err := repository.git(removeArguments...); err != nil {
		return worktreeRemovalResult{}, err
	}
	if standardLayout {
		if err := removeEmptyParents(worktree.Path, runtime.HomeDirectory); err != nil {
			return worktreeRemovalResult{}, err
		}
	}

	if err := deleteRemovedBranch(repository, worktree, force); err != nil {
		return worktreeRemovalResult{}, err
	}

	return worktreeRemovalResult{worktree: worktree, removingCurrentDirectory: removingCurrentDirectory}, nil
}

func (x Runtime) repairStaleWorktrees(repo registeredRepo) (bool, error) {
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return false, err
	}
	worktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return false, err
	}

	stale, err := collectStaleWorktrees(x, repo, worktrees)
	if err != nil {
		return false, err
	}
	if len(stale) == 0 {
		return false, nil
	}
	if err := repairWorktrees(x, repo.BarePath, stale); err != nil {
		return false, err
	}
	for _, worktreePath := range stale {
		if err := validateLinkedWorktree(x, worktreePath, repo.BarePath); err != nil {
			return false, fmt.Errorf("verify repaired worktree: %w", err)
		}
	}
	return true, nil
}

func (x Runtime) reportRenamedCurrentPath(path string) error {
	pathFile := x.RenamePathFile
	if pathFile == "" || isEmpty(path) {
		return nil
	}
	if err := x.writePathFile(pathFile, path); err != nil {
		return fmt.Errorf("write renamed worktree path file: %w", err)
	}
	return nil
}

func (x Runtime) resolveCreateWorktreeName(repoName string, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	return x.unusedWorktreeName(repoName)
}

func (x Runtime) runGh(ctx context.Context, directory string, args ...string) ([]byte, error) {
	command := x.command(ctx, "gh", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	operation := strings.Join(args[:min(2, len(args))], " ")
	message := strings.TrimSpace(string(output))
	if message == "" {
		return nil, fmt.Errorf("gh %s: %w", operation, err)
	}
	return nil, fmt.Errorf("gh %s: %w: %s", operation, err, message)
}

func (x Runtime) runHerdr(ctx context.Context, args ...string) ([]byte, error) {
	command := x.command(ctx, "herdr", args...)
	output, err := command.CombinedOutput()
	if err == nil {
		return output, nil
	}

	operation := strings.Join(args[:min(2, len(args))], " ")
	message := strings.TrimSpace(string(output))
	if message == "" {
		return nil, fmt.Errorf("herdr %s: %w", operation, err)
	}
	return nil, fmt.Errorf("herdr %s: %w: %s", operation, err, message)
}

func (x Runtime) selectManagedWorktree(worktrees []managedWorktree, name string) (managedWorktree, error) {
	if name != "" {
		return managedWorktreeByName(worktrees, name)
	}

	currentDirectory := x.CurrentDirectory

	currentRepository, err := openRepository(x, currentDirectory)
	if err != nil {
		return managedWorktree{}, fmt.Errorf("worktree name is required when not inside a managed worktree: %w", err)
	}

	return managedWorktreeForPath(worktrees, currentRepository.WorkTree)
}

func (x Runtime) setupHerdrParentDashboard(ctx context.Context, output []byte, repoName string, barePath string) (string, error) {
	space, err := parseHerdrSpace(x, output, barePath, repoName)
	if err != nil {
		return "", err
	}
	if _, err := x.runHerdr(ctx, "tab", "rename", space.agentTabID, parentDashboardTabLabel); err != nil {
		return "", err
	}
	dashboard := parentDashboardCommand(repoName, x.ghAuthOK(ctx))
	if _, err := x.runHerdr(ctx, "pane", "run", space.agentPaneID, dashboard); err != nil {
		return "", err
	}
	return space.workspaceID, nil
}

// todoProgressForWorktree counts the worktree's TODO.md checklist items. A
// worktree whose checklist cannot be read reports no progress; list already
// degrades to placeholder cells for unreadable worktree state.
func (x Runtime) todoProgressForWorktree(worktree managedWorktree) todo.Progress {
	gitDir, err := x.worktreeGitDir(worktree.Path)
	if err != nil {
		return todo.Progress{}
	}
	progress, err := todo.ReadProgress(gitDir)
	if err != nil {
		return todo.Progress{}
	}
	return progress
}

// trashExecutable returns the command used to move paths to the system trash.
func (x Runtime) trashExecutable() string {
	return cmp.Or(x.TrashExecutable, trashCommandName)
}

// trashPaths moves paths to the system trash with the trash CLI.
func (x Runtime) trashPaths(paths ...string) error {
	trashablePaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" {
			trashablePaths = append(trashablePaths, path)
		}
	}
	if len(trashablePaths) == 0 {
		return nil
	}

	command := x.command(context.Background(), x.trashExecutable(), trashablePaths...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf(
			"trash %s: %w: %s",
			strings.Join(trashablePaths, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return nil
}

func (x Runtime) unusedWorktreeName(repoName string) (string, error) {
	return firstUnusedWorktreeName(randomWorktreeName, func(name string) (bool, error) {
		return x.worktreeDirectoryExists(repoName, name)
	})
}

// validateTrashCommand fails when the trash CLI is not available so the
// import can stop before mutating anything.
func (x Runtime) validateTrashCommand() error {
	executable := x.trashExecutable()
	if _, err := exec.LookPath(executable); err != nil {
		return fmt.Errorf(
			"trash command %q not found; install a trash CLI (e.g. trash-cli) so old worktrees are removed to the system trash instead of deleted",
			executable,
		)
	}
	return nil
}

func (x Runtime) worktreeDirectoryExists(repoName string, name string) (bool, error) {
	path := x.managedWorktreePath(repoName, name)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect worktree directory %q: %w", path, err)
	}
	repository, _, err := x.openRegisteredRepository(repoName)
	if err != nil {
		return false, err
	}
	worktrees, err := x.managedWorktreesFromRepository(repository, repoName)
	if err != nil {
		return false, err
	}
	_, err = managedWorktreeByName(worktrees, name)
	return err == nil, nil
}

// worktreeGitDir resolves the Git directory backing a checkout, which is where
// worktree-scoped state such as TODO.md lives.
func (x Runtime) worktreeGitDir(path string) (string, error) {
	result, err := gitOutput(x, path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("resolve Git directory for %q: %w", path, err)
	}
	return filepath.Clean(result.stdout), nil
}

func (x Runtime) worktreeReviewProgress(ctx context.Context, path string) *reviewProgress {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	service := review.Service{Repository: review.Repository{Dir: path, Environment: x.Environment}}
	result, err := service.Status(ctx, false)
	if err != nil || !isSame(result.Version, 1) || !isSame(result.Command, "status") {
		return nil
	}
	progress := new(reviewProgress)
	for _, entry := range result.Summary {
		progress.Total += entry.Count
		if entry.State == "approved" {
			progress.Approved = entry.Count
		}
	}
	return progress
}

func (x Runtime) worktreeRoot() string {
	return x.WorktreeRoot
}

func (x Runtime) worktreesForRepo(repo registeredRepo) ([]managedWorktree, error) {
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return nil, err
	}
	return x.managedWorktreesFromRepository(repository, repo.Name)
}

func (runtime Runtime) worktreesToListTable(worktrees []managedWorktree, location bool) []ListWorktreeView {
	views := make([]ListWorktreeView, 0, len(worktrees))
	for _, worktree := range worktrees {
		view := ListWorktreeView{
			Name:            worktree.Name,
			Repo:            worktree.Repo,
			Ahead:           worktree.ListStatus.Ahead,
			Behind:          worktree.ListStatus.Behind,
			Upstream:        worktree.ListStatus.Upstream,
			DefaultUpstream: worktree.DefaultUpstream,
			Merged:          worktree.Merged,
			StatusError:     worktree.ListError,
			Todo:            worktree.Todo.String(),
			Review:          worktree.Review.String(),
			Commit:          worktree.shortCommitHash(),
			Clean:           worktree.Clean,
			PullRequest:     worktree.PullRequest,
		}
		if location {
			view.DisplayPath = runtime.displayHomePath(worktree.Path)
		}
		views = append(views, view)
	}
	return views
}

func (x Runtime) writePathFile(pathFile string, value string) (err error) {
	temporaryDirectory, err := x.absolutePath(x.TemporaryDirectory)
	if err != nil {
		return fmt.Errorf("resolve temporary directory: %w", err)
	}
	temporaryDirectory, err = filepath.EvalSymlinks(temporaryDirectory)
	if err != nil {
		return fmt.Errorf("resolve temporary directory: %w", err)
	}
	pathFile, err = x.absolutePath(pathFile)
	if err != nil {
		return fmt.Errorf("resolve path file: %w", err)
	}
	pathFileDirectory, err := filepath.EvalSymlinks(filepath.Dir(pathFile))
	if err != nil {
		return fmt.Errorf("resolve path file: %w", err)
	}
	pathFile = filepath.Join(pathFileDirectory, filepath.Base(pathFile))

	relativePath, err := filepath.Rel(temporaryDirectory, pathFile)
	if err != nil {
		return fmt.Errorf("relate path file to temporary directory: %w", err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path file %q is outside temporary directory", pathFile)
	}

	root, err := os.OpenRoot(temporaryDirectory)
	if err != nil {
		return fmt.Errorf("open temporary directory: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); err == nil {
			err = closeErr
		}
	}()

	return root.WriteFile(relativePath, []byte(value+"\n"), 0o600)
}

func (x Runtime) xdgConfigHome() string {
	return x.ConfigHome
}

func (x Runtime) xdgDataHome() string {
	return x.DataHome
}

// RuntimeFromProcess captures the process state needed by a timber command.
func RuntimeFromProcess() (Runtime, error) {
	currentDirectory, err := os.Getwd()
	if err != nil {
		return Runtime{}, fmt.Errorf("get current directory: %w", err)
	}

	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return Runtime{}, fmt.Errorf("resolve home directory: %w", err)
	}

	return Runtime{
		CurrentDirectory:   currentDirectory,
		HomeDirectory:      homeDirectory,
		DataHome:           cmp.Or(os.Getenv("XDG_DATA_HOME"), filepath.Join(homeDirectory, ".local", "share")),
		ConfigHome:         cmp.Or(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(homeDirectory, ".config")),
		WorktreeRoot:       cmp.Or(os.Getenv(worktreeRootEnvVarName), filepath.Join(homeDirectory, worktreesDirName)),
		TemporaryDirectory: os.TempDir(),
		HerdrEnvironment:   os.Getenv("HERDR_ENV") == "1",
		CreatePathFile:     os.Getenv(createPathFileEnvVarName),
		SwitchPathFile:     os.Getenv(switchPathFileEnvVarName),
		RenamePathFile:     os.Getenv(repoRenamePathFileEnvVarName),
		Environment:        slices.Clone(os.Environ()),
	}, nil
}
