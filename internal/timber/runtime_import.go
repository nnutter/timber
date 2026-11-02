package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func openImportSource(x Runtime, sourcePath string) (*Repository, error) {
	sourceRepository, err := openRepository(x, sourcePath)
	if err != nil {
		return nil, importOpenError(x, sourcePath, err)
	}
	return sourceRepository, nil
}

func importOpenError(x Runtime, sourcePath string, err error) error {
	bare, bareErr := pathIsBareRepository(x, sourcePath)
	if bare && isSame(bareErr, nil) {
		display := x.displayHomePath(sourcePath)
		return fmt.Errorf("%s is a bare repository; use 'timber repo add %s' instead", display, display)
	}
	return fmt.Errorf("open repository at %s: %w", x.displayHomePath(sourcePath), err)
}

func importPlanForRepository(x Runtime, sourceRepository *Repository, requestedName string) (importPlan, error) {
	mainPath, porcelainWorktrees, err := importMainPath(x, sourceRepository)
	if err != nil {
		return importPlan{}, err
	}
	// Resolve the canonical main checkout so subdirectory or linked-worktree
	// arguments still import the whole repository.
	sourceRepository, err = openRepository(x, mainPath)
	if err != nil {
		return importPlan{}, err
	}
	plan, err := newImportPlan(x, sourceRepository, requestedName, mainPath)
	if err != nil {
		return importPlan{}, err
	}
	if err := plan.collectWorktrees(porcelainWorktrees); err != nil {
		return importPlan{}, err
	}
	if err := plan.validateTargets(); err != nil {
		return importPlan{}, err
	}
	if err := x.validateTrashCommand(); err != nil {
		return importPlan{}, err
	}
	return plan, nil
}

func importMainPath(x Runtime, sourceRepository *Repository) (string, []porcelainWorktree, error) {
	commonDir, err := sourceRepository.commonGitDir()
	if err != nil {
		return "", nil, err
	}
	if err := rejectRegisteredSource(x, commonDir); err != nil {
		return "", nil, err
	}
	porcelainWorktrees, err := sourceRepository.listPorcelainWorktrees()
	if err != nil {
		return "", nil, err
	}
	if len(porcelainWorktrees) == 0 {
		return "", nil, errors.New("repository has no worktrees")
	}
	// A bare repository with linked worktrees lists itself first. When the
	// Git directory is named .git, that path is the parent, not commonDir.
	mainEntry := porcelainWorktrees[0]
	bareMain := mainEntry.Bare
	if !bareMain {
		bareMain, err = samePath(mainEntry.Path, commonDir)
		if err != nil {
			return "", nil, err
		}
	}
	if bareMain {
		display := x.displayHomePath(mainEntry.Path)
		return "", nil, fmt.Errorf("%s is a bare repository; use 'timber repo add %s' instead", display, display)
	}
	return mainEntry.Path, porcelainWorktrees, nil
}

func newImportPlan(x Runtime, sourceRepository *Repository, requestedName, mainPath string) (importPlan, error) {
	repoName := requestedName
	if repoName == "" {
		repoName = defaultRepoNameForImport(sourceRepository, mainPath)
	}
	if err := validateRepoName(repoName); err != nil {
		return importPlan{}, err
	}
	barePath := x.bareRepoPath(repoName)
	if _, err := os.Stat(barePath); err == nil {
		return importPlan{}, fmt.Errorf("repository %q already exists at %s", repoName, barePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return importPlan{}, fmt.Errorf("inspect repository path %q: %w", barePath, err)
	}
	return importPlan{
		runtime:  x,
		source:   sourceRepository,
		repoName: repoName,
		barePath: barePath,
		mainPath: mainPath,
	}, nil
}

func relocatedWorktreePath(x Runtime, repo registeredRepo, oldPath string) (string, bool, error) {
	oldBare := filepath.Join(x.legacyReposDirectory(), filepath.FromSlash(repo.Name)+bareRepoSuffix)
	rel, err := filepath.Rel(oldBare, oldPath)
	if err != nil || isSame(rel, ".") || !pathIsWithin(oldBare, oldPath) {
		return "", false, nil
	}
	path := filepath.Join(repo.BarePath, rel)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() || isSymlink(info) {
		return "", false, nil
	}
	return path, true, nil
}

func matchingMovedGitDir(x Runtime, repo registeredRepo, oldPath, path string) (string, error) {
	content, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return "", nil
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir: ")
	if !ok {
		return "", nil
	}
	oldBare := filepath.Join(x.legacyReposDirectory(), filepath.FromSlash(repo.Name)+bareRepoSuffix)
	oldAdmin := filepath.Join(oldBare, "worktrees")
	adminRel, err := filepath.Rel(oldAdmin, filepath.Clean(gitdir))
	if err != nil || isSame(adminRel, ".") || isSame(adminRel, "..") || !isSame(filepath.Base(adminRel), adminRel) {
		return "", nil
	}
	backlink, err := os.ReadFile(filepath.Join(repo.BarePath, "worktrees", adminRel, "gitdir"))
	if err != nil || !isSame(filepath.Clean(strings.TrimSpace(string(backlink))), filepath.Join(oldPath, ".git")) {
		return "", nil
	}
	if _, err := os.Lstat(oldPath); !errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return path, nil
}

func refuseUnsafeRemoval(runtime Runtime, repository *Repository, worktree managedWorktree, name string, force bool) (managedWorktree, error) {
	if force {
		return worktree, nil
	}
	if err := repository.fetchOrigin(); err != nil {
		return managedWorktree{}, err
	}
	worktree, err := runtime.enrichManagedWorktree(repository, worktree)
	if err != nil {
		return managedWorktree{}, err
	}
	if !worktree.Clean {
		return managedWorktree{}, fmt.Errorf("worktree %q is not clean", name)
	}
	if worktree.UpstreamRef == "" {
		return managedWorktree{}, fmt.Errorf("branch %q has no upstream branch; use --force to remove it", worktree.branchName())
	}
	if !worktree.Merged {
		return managedWorktree{}, fmt.Errorf("branch %q is not merged to %s", worktree.branchName(), shortReference(worktree.UpstreamRef))
	}
	return worktree, nil
}

func deleteRemovedBranch(repository *Repository, worktree managedWorktree, force bool) error {
	branchExists, err := repository.branchStillExists(worktree.BranchReference)
	if err != nil {
		return err
	}
	if !branchExists {
		return nil
	}
	_, err = repository.git("branch", branchDeleteFlag(force), worktree.branchName())
	return err
}

func collectStaleWorktrees(x Runtime, repo registeredRepo, worktrees []porcelainWorktree) ([]string, error) {
	stale := make([]string, 0)
	for _, worktree := range worktrees {
		path, include, err := staleWorktreePath(x, repo, worktree)
		if err != nil {
			return nil, err
		}
		if include {
			stale = append(stale, path)
		}
	}
	return stale, nil
}

func staleWorktreePath(x Runtime, repo registeredRepo, worktree porcelainWorktree) (string, bool, error) {
	if worktree.Bare {
		return "", false, nil
	}
	if worktree.Prunable != "" {
		path, err := x.movedInternalWorktree(repo, worktree.Path)
		return path, path != "", err
	}
	needsRepair, err := worktreeGitDirIsStale(worktree.Path, repo.BarePath)
	if err != nil {
		return "", false, err
	}
	return worktree.Path, needsRepair, nil
}

func switchWorktreePath(runtime Runtime, repoName, name string) (string, error) {
	repository, _, err := runtime.openRegisteredRepository(repoName)
	if err != nil {
		return "", err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, repoName)
	if err != nil {
		return "", err
	}
	worktree, err := managedWorktreeByName(worktrees, name)
	if err != nil {
		return "", fmt.Errorf("worktree %s not found in repository %s", name, repoName)
	}
	if err := requireWorktreeDirectory(name, worktree.Path); err != nil {
		return "", err
	}
	return worktree.Path, nil
}

func pullRequestRemoteParts(repository *Repository, worktree managedWorktree, head string) (string, string, string, error) {
	remote, err := repository.pullRequestRemote(head)
	if err != nil {
		return "", "", "", err
	}
	remoteURL, configured, err := repository.gitConfigValue("remote." + remote + ".url")
	if err != nil {
		return "", "", "", err
	}
	if !configured || isEmpty(remoteURL) {
		return "", "", "", fmt.Errorf("repository %s has no %s remote; a pull request needs a GitHub remote", worktree.Repo, remote)
	}
	owner, repo, err := githubRepositoryFromRemoteURL(remoteURL)
	return remote, owner, repo, err
}
