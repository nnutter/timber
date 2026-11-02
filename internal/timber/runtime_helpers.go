package timber

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nnutter/timber/internal/terminal"
	coreconfig "github.com/nnutter/timber/internal/timber/config"
)

func repoNameForAdd(name, remoteURL string) (string, error) {
	repoName := normalizeRepoName(name)
	if repoName == "" {
		var err error
		repoName, err = defaultRepoNameFromRemote(remoteURL)
		if err != nil {
			return "", err
		}
	}
	if err := validateRepoName(repoName); err != nil {
		return "", err
	}
	return repoName, nil
}

func rejectExistingRepo(targetPath, repoName string) error {
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}
	return nil
}

func cloneBareRepo(runtime Runtime, remoteURL, targetPath, alias string) error {
	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return err
	}
	if _, err := gitOutput(runtime, runtime.CurrentDirectory, "clone", "--bare", remoteURL, targetPath); err != nil {
		return err
	}
	if err := configureBareOriginTracking(runtime, targetPath); err != nil {
		return err
	}
	return writeRepoAlias(runtime, targetPath, alias)
}

func writeRepoAlias(runtime Runtime, targetPath, alias string) error {
	if alias == "" {
		return nil
	}
	if _, err := gitOutput(runtime, targetPath, "config", "--local", "timber.alias", alias); err != nil {
		return err
	}
	return nil
}

func attachPullRequests(runtime Runtime, ctx context.Context, worktrees []managedWorktree, pullRequests bool) error {
	if !pullRequests {
		return nil
	}
	return runtime.enrichListedWorktreesWithPullRequests(ctx, worktrees)
}

func renderWorktreeList(runtime Runtime, stdout io.Writer, worktrees []managedWorktree, pullRequests, location, jsonOutput bool, renderers ListRenderers) error {
	if jsonOutput {
		if renderers.JSON == nil {
			return errors.New("list JSON renderer is not configured")
		}
		return renderers.JSON(stdout, worktreesToListJSON(worktrees))
	}
	if renderers.Table == nil {
		return errors.New("list table renderer is not configured")
	}
	return renderers.Table(stdout, runtime.worktreesToListTable(worktrees, location), pullRequests, location)
}

func validateCreateReporters(runtime Runtime, herdr, noHerdr bool, reporters CreateReporters) error {
	if reporters.Created == nil {
		return errors.New("create requires a worktree creation reporter")
	}
	if opensHerdr(runtime, herdr, noHerdr) && isNilFunc(reporters.HerdrOpened) {
		return errors.New("create requires a Herdr opening reporter")
	}
	return nil
}

func opensHerdr(runtime Runtime, herdr, noHerdr bool) bool {
	return herdr || (!noHerdr && isSame(runtime.HerdrEnvironment, true))
}

type createdWorktree struct {
	repo       string
	branch     string
	path       string
	upstream   string
	repository *Repository
}

func createWorktreeCheckout(runtime Runtime, input io.Reader, repoName, raw, explicitPath string, hasExplicitPath bool, upstream string) (createdWorktree, error) {
	repo, repository, branchName, err := resolveCreateBranch(runtime, input, repoName, raw)
	if err != nil {
		return createdWorktree{}, err
	}
	worktreePath, err := createWorktreePath(runtime, repo.Name, branchName, explicitPath, hasExplicitPath)
	if err != nil {
		return createdWorktree{}, err
	}
	if err := rejectDuplicateWorktree(runtime, repository, repo.Name, branchName, worktreePath); err != nil {
		return createdWorktree{}, err
	}
	upstreamBranch, err := addCreateBranch(runtime, repository, branchName, worktreePath, upstream, hasExplicitPath)
	if err != nil {
		return createdWorktree{}, err
	}
	return createdWorktree{repo: repo.Name, branch: branchName, path: worktreePath, upstream: upstreamBranch, repository: repository}, nil
}

func resolveCreateBranch(runtime Runtime, input io.Reader, repoName, raw string) (registeredRepo, *Repository, string, error) {
	qualified, err := runtime.parseQualifiedName(raw)
	if err != nil {
		return registeredRepo{}, nil, "", err
	}
	if err := ValidateWorktreeName(qualified.Name); err != nil {
		return registeredRepo{}, nil, "", err
	}
	if qualified.Repo != "" {
		repoName = qualified.Repo
	}
	selection := repoSelection{runtime: runtime, RepoName: repoName}
	repo, repository, err := selection.resolve(input)
	if err != nil {
		return registeredRepo{}, nil, "", err
	}
	branchName, err := runtime.resolveCreateWorktreeName(repo.Name, qualified.Name)
	if err != nil {
		return registeredRepo{}, nil, "", err
	}
	return repo, repository, branchName, nil
}

func createWorktreePath(runtime Runtime, repoName, branchName, explicitPath string, hasExplicitPath bool) (string, error) {
	worktreePath := runtime.managedWorktreePath(repoName, branchName)
	if !hasExplicitPath {
		return worktreePath, nil
	}
	if explicitPath == "" {
		return "", errors.New("worktree path must not be empty")
	}
	return runtime.absolutePath(explicitPath)
}

func rejectDuplicateWorktree(runtime Runtime, repository *Repository, repoName, branchName, worktreePath string) error {
	if err := rejectNonEmptyWorktreeDirectory(worktreePath); err != nil {
		return err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, repoName)
	if err != nil {
		return err
	}
	if _, err := managedWorktreeByName(worktrees, branchName); err == nil {
		return fmt.Errorf("worktree %q already exists", branchName)
	}
	return nil
}

func addCreateBranch(runtime Runtime, repository *Repository, branchName, worktreePath, upstream string, hasExplicitPath bool) (string, error) {
	if err := repository.fetchOrigin(); err != nil {
		return "", err
	}
	upstreamBranch, err := createUpstream(repository, upstream)
	if err != nil {
		return "", err
	}
	if err := addWorktreeBranch(repository, branchName, worktreePath, upstreamBranch); err != nil {
		return "", err
	}
	if err := repository.setBranchUpstream(branchName, upstreamBranch); err != nil {
		return "", err
	}
	if err := recordExplicitWorktreeName(runtime, worktreePath, branchName, hasExplicitPath); err != nil {
		return "", err
	}
	return upstreamBranch, nil
}

func createUpstream(repository *Repository, upstream string) (string, error) {
	if upstream != "" {
		return upstream, nil
	}
	return repository.remoteHeadBranch()
}

func addWorktreeBranch(repository *Repository, branchName, worktreePath, upstreamBranch string) error {
	branchExists, err := repository.branchExists(branchName)
	if err != nil {
		return err
	}
	if err := ensureWorktreeDirectory(worktreePath); err != nil {
		return err
	}
	if branchExists {
		_, err = repository.git("worktree", "add", worktreePath, branchName)
		return err
	}
	_, err = repository.git("worktree", "add", "-b", branchName, worktreePath, upstreamBranch)
	return err
}

func recordExplicitWorktreeName(runtime Runtime, worktreePath, branchName string, hasExplicitPath bool) error {
	if !hasExplicitPath {
		return nil
	}
	gitDir, err := runtime.worktreeGitDir(worktreePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(gitDir, worktreeNameFile), []byte(branchName+"\n"), 0o600); err != nil {
		return fmt.Errorf("record worktree name: %w", err)
	}
	return nil
}

func openCreatedHerdr(ctx context.Context, runtime Runtime, output io.Writer, created createdWorktree, herdr, noHerdr bool, reporters CreateReporters) (string, bool, error) {
	if !opensHerdr(runtime, herdr, noHerdr) {
		return created.path, false, nil
	}
	worktree := managedWorktree{Repo: created.repo, Name: created.branch, Path: created.path}
	if err := runtime.openHerdrSpace(ctx, worktree); err != nil {
		return "", false, err
	}
	if err := reporters.HerdrOpened(output, created.branch); err != nil {
		return "", false, err
	}
	return created.path, true, nil
}

func validatePruneReporters(prompt, dryRun bool, prompter WorktreePrompter, reporters PruneReporters) error {
	if prompter == nil && isTrue(prompt) {
		return errors.New("prune prompt requires a worktree prompter")
	}
	if dryRun && isNilFunc(reporters.DryRun) {
		return errors.New("prune dry run requires a reporter")
	}
	if !dryRun && isNilFunc(reporters.Removal) {
		return errors.New("prune removal requires a reporter")
	}
	return nil
}

func pruneCandidates(runtime Runtime, repoArg string) ([]managedWorktree, error) {
	selection := repoSelection{runtime: runtime}
	if repoArg != "" {
		repo, err := runtime.parseRepoOnlyArg(repoArg)
		if err != nil {
			return nil, err
		}
		selection.RepoName = repo
	}
	repos, err := selection.reposToConsider()
	if err != nil {
		return nil, err
	}
	return runtime.collectManagedWorktrees(repos)
}

func selectPruneWorktrees(input io.Reader, output io.Writer, enriched []managedWorktree, prompt bool, prompter WorktreePrompter) ([]managedWorktree, error) {
	if prompter != nil && isTrue(prompt) {
		return promptPruneWorktrees(input, output, enriched, prompter)
	}
	return cleanMergedWorktrees(enriched), nil
}

func promptPruneWorktrees(input io.Reader, output io.Writer, enriched []managedWorktree, prompter WorktreePrompter) ([]managedWorktree, error) {
	refs := make([]WorktreeRef, 0, len(enriched))
	for _, worktree := range enriched {
		refs = append(refs, worktree.ref())
	}
	selectedRefs, err := prompter.Prompt(input, output, refs)
	if err != nil {
		return nil, err
	}
	selected := make([]managedWorktree, 0, len(selectedRefs))
	for _, ref := range selectedRefs {
		worktree, err := managedWorktreeByKey(enriched, worktreeRefKey(ref))
		if err != nil {
			return nil, err
		}
		selected = append(selected, worktree)
	}
	return selected, nil
}

func cleanMergedWorktrees(worktrees []managedWorktree) []managedWorktree {
	selected := make([]managedWorktree, 0)
	for _, worktree := range worktrees {
		if cleanMergedWorktree(worktree) {
			selected = append(selected, worktree)
		}
	}
	return selected
}

func cleanMergedWorktree(worktree managedWorktree) bool {
	return worktree.Clean && isSame(worktree.Merged, true)
}

func finishPrune(runtime Runtime, output io.Writer, selected []managedWorktree, prompt, dryRun bool, reporters PruneReporters) error {
	if dryRun {
		return reportPruneDryRun(runtime, output, selected, reporters)
	}
	return removePruneSelection(runtime, output, selected, prompt, reporters)
}

func reportPruneDryRun(runtime Runtime, output io.Writer, selected []managedWorktree, reporters PruneReporters) error {
	for _, worktree := range selected {
		if err := reporters.DryRun(output, worktree.Name, worktree.Repo, runtime.displayHomePath(worktree.Path)); err != nil {
			return err
		}
	}
	return nil
}

func linkedWorktreeCount(repository *Repository, barePath string) (int, error) {
	porcelain, err := repository.listPorcelainWorktrees()
	if err != nil {
		return 0, err
	}
	linked := 0
	for _, worktree := range porcelain {
		if worktree.Bare {
			continue
		}
		same, err := samePath(worktree.Path, barePath)
		if err != nil {
			return 0, err
		}
		if same {
			continue
		}
		linked++
	}
	return linked, nil
}

func requireWorktreeDirectory(name, path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("worktree %s not found at %s", name, path)
		}
		return fmt.Errorf("inspect worktree directory %q: %w", path, err)
	}
	return nil
}

type legacyRepoWalk struct {
	runtime   Runtime
	directory string
	repos     []registeredRepo
}

func (walk *legacyRepoWalk) visit(path string, entry os.DirEntry, err error) error {
	if err != nil || !entry.IsDir() || isSame(path, walk.directory) {
		return nil
	}
	if !strings.HasSuffix(entry.Name(), bareRepoSuffix) {
		return nil
	}
	bare, bareErr := pathIsBareRepository(walk.runtime, path)
	if !bare || !isSame(bareErr, nil) {
		return filepath.SkipDir
	}
	relative, relErr := filepath.Rel(walk.directory, path)
	if relErr != nil {
		return filepath.SkipDir
	}
	repoName := filepath.ToSlash(strings.TrimSuffix(relative, bareRepoSuffix))
	if err := validateRepoName(repoName); err != nil {
		return filepath.SkipDir
	}
	walk.repos = append(walk.repos, registeredRepo{Name: repoName, BarePath: path})
	return filepath.SkipDir
}

func reportSetupWarning(output io.Writer, reportWarning func(io.Writer, SetupKind, error) error, kind SetupKind, err error) {
	if err == nil || isNilFunc(reportWarning) {
		return
	}
	_ = reportWarning(output, kind, err)
}

func installHKHooks(x Runtime, ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter) error {
	settings, identity, eligible, err := hkInstallEligible(x, ctx, input, repository, upstream, worktreePath)
	if err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	enabled, err := confirmHKInstall(x, input, output, settings, identity, prompter)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return runHKInstall(x, ctx, worktreePath)
}

func hkInstallEligible(x Runtime, ctx context.Context, input io.Reader, repository *Repository, upstream, worktreePath string) (coreconfig.Settings, githubIdentity, bool, error) {
	settings, err := x.loadConfig()
	if err != nil {
		return coreconfig.Settings{}, githubIdentity{}, false, err
	}
	if !settings.AutoHKInstall && (settings.AutoHKInstallDecided || !terminal.IsInteractive(input)) {
		return settings, githubIdentity{}, false, nil
	}
	regular, err := regularRootFile(filepath.Join(worktreePath, "hk.pkl"))
	if err != nil {
		return settings, githubIdentity{}, false, err
	}
	if !regular {
		return settings, githubIdentity{}, false, nil
	}
	identity, owned, err := x.githubOwnedWorktree(ctx, repository, upstream)
	if err != nil {
		return settings, githubIdentity{}, false, err
	}
	if !owned {
		return settings, githubIdentity{}, false, nil
	}
	return settings, identity, true, nil
}

func confirmHKInstall(x Runtime, input io.Reader, output io.Writer, settings coreconfig.Settings, identity githubIdentity, prompter SetupPrompter) (bool, error) {
	if settings.AutoHKInstall {
		return true, nil
	}
	if prompter == nil {
		return false, errors.New("automatic setup prompt is not configured")
	}
	enabled, err := prompter.Choose(input, output, SetupHKInstall, SetupIdentity{Username: identity.Username, Organizations: identity.Organizations})
	if err != nil {
		return false, err
	}
	settings.AutoHKInstall = enabled
	settings.AutoHKInstallDecided = true
	if err := writeConfig(x.configPath(), settings); err != nil {
		return false, fmt.Errorf("save hk install choice: %w", err)
	}
	if !enabled {
		err = prompter.ReportDisabled(output, SetupHKInstall)
		return false, err
	}
	return true, nil
}

func runHKInstall(x Runtime, ctx context.Context, worktreePath string) error {
	args := []string{"install"}
	hasMise, err := regularRootFile(filepath.Join(worktreePath, "mise.toml"))
	if err != nil {
		return err
	}
	if hasMise {
		args = append(args, "--mise")
	}
	install := x.command(ctx, "hk", args...)
	install.Dir = worktreePath
	combined, err := install.CombinedOutput()
	if err != nil {
		return fmt.Errorf("hk install failed: %w: %s", err, strings.TrimSpace(string(combined)))
	}
	return nil
}

func trustMiseConfig(x Runtime, ctx context.Context, input io.Reader, output io.Writer, repository *Repository, upstream, worktreePath string, prompter SetupPrompter) error {
	settings, identity, configPath, eligible, err := miseTrustEligible(x, ctx, input, repository, upstream, worktreePath)
	if err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	enabled, err := confirmMiseTrust(x, input, output, settings, identity, prompter)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return runMiseTrust(x, ctx, worktreePath, configPath)
}

func miseTrustEligible(x Runtime, ctx context.Context, input io.Reader, repository *Repository, upstream, worktreePath string) (coreconfig.Settings, githubIdentity, string, bool, error) {
	settings, err := x.loadConfig()
	if err != nil {
		return coreconfig.Settings{}, githubIdentity{}, "", false, err
	}
	if !settings.AutoMiseTrust && (settings.AutoMiseTrustDecided || !terminal.IsInteractive(input)) {
		return settings, githubIdentity{}, "", false, nil
	}
	configPath := filepath.Join(worktreePath, "mise.toml")
	regular, err := regularRootFile(configPath)
	if err != nil {
		return settings, githubIdentity{}, configPath, false, err
	}
	// Do not trust a symlink that can point outside the new checkout.
	if !regular {
		return settings, githubIdentity{}, configPath, false, nil
	}
	identity, owned, err := x.githubOwnedWorktree(ctx, repository, upstream)
	if err != nil {
		return settings, githubIdentity{}, configPath, false, err
	}
	if !owned {
		return settings, githubIdentity{}, configPath, false, nil
	}
	return settings, identity, configPath, true, nil
}

func confirmMiseTrust(x Runtime, input io.Reader, output io.Writer, settings coreconfig.Settings, identity githubIdentity, prompter SetupPrompter) (bool, error) {
	if settings.AutoMiseTrust {
		return true, nil
	}
	if prompter == nil {
		return false, errors.New("automatic setup prompt is not configured")
	}
	enabled, err := prompter.Choose(input, output, SetupMiseTrust, SetupIdentity{Username: identity.Username, Organizations: identity.Organizations})
	if err != nil {
		return false, err
	}
	settings.AutoMiseTrust = enabled
	settings.AutoMiseTrustDecided = true
	if err := writeConfig(x.configPath(), settings); err != nil {
		return false, fmt.Errorf("save mise trust choice: %w", err)
	}
	if !enabled {
		err = prompter.ReportDisabled(output, SetupMiseTrust)
		return false, err
	}
	return true, nil
}

func runMiseTrust(x Runtime, ctx context.Context, worktreePath, configPath string) error {
	trust := x.command(ctx, "mise", "trust", configPath)
	trust.Dir = worktreePath
	combined, err := trust.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mise trust failed: %w: %s", err, strings.TrimSpace(string(combined)))
	}
	return nil
}

func removePruneSelection(runtime Runtime, output io.Writer, selected []managedWorktree, prompt bool, reporters PruneReporters) error {
	for _, worktree := range selected {
		if !prompt && (!worktree.Clean || !isSame(worktree.Merged, true)) {
			continue
		}
		// Removing a checkout can shorten other directory-derived names.
		// Use the selected identity rather than resolving its name again.
		repository, _, err := runtime.openRegisteredRepository(worktree.Repo)
		if err != nil {
			return err
		}
		result, err := runtime.removeResolvedWorktree(repository, worktree, true)
		if err != nil {
			return err
		}
		if err := reporters.Removal(output, result.worktree.Name, result.worktree.shortCommitHash(), result.worktree.Path, result.removingCurrentDirectory); err != nil {
			return err
		}
	}
	return nil
}
