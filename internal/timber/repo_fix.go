package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const legacyReposDirName = "timber/repos"

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

	repos := make([]registeredRepo, 0)
	walkErr := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == directory {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), bareRepoSuffix) {
			return nil
		}
		bare, bareErr := pathIsBareRepository(x, path)
		if bareErr != nil || !bare {
			return filepath.SkipDir
		}
		relative, relErr := filepath.Rel(directory, path)
		if relErr != nil {
			return filepath.SkipDir
		}
		repoName := filepath.ToSlash(strings.TrimSuffix(relative, bareRepoSuffix))
		if err := validateRepoName(repoName); err != nil {
			return filepath.SkipDir
		}
		repos = append(repos, registeredRepo{Name: repoName, BarePath: path})
		return filepath.SkipDir
	})
	if walkErr != nil {
		return nil, fmt.Errorf("read legacy repos directory %q: %w", directory, walkErr)
	}
	slices.SortFunc(repos, func(left, right registeredRepo) int {
		return strings.Compare(left.Name, right.Name)
	})
	return repos, nil
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

func (x Runtime) repairStaleWorktrees(repo registeredRepo) (bool, error) {
	repository, err := openBareRepository(x, repo.BarePath)
	if err != nil {
		return false, err
	}
	worktrees, err := repository.listPorcelainWorktrees()
	if err != nil {
		return false, err
	}

	stale := make([]string, 0)
	for _, worktree := range worktrees {
		if worktree.Bare {
			continue
		}
		if worktree.Prunable != "" {
			path, err := x.movedInternalWorktree(repo, worktree.Path)
			if err != nil {
				return false, err
			}
			if path != "" {
				stale = append(stale, path)
			}
			continue
		}
		needsRepair, err := worktreeGitDirIsStale(worktree.Path, repo.BarePath)
		if err != nil {
			return false, err
		}
		if needsRepair {
			stale = append(stale, worktree.Path)
		}
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

// movedInternalWorktree recognizes a checkout that moved with a legacy bare
// repository. Both old links must agree before Git can repair either one.
func (x Runtime) movedInternalWorktree(repo registeredRepo, oldPath string) (string, error) {
	oldBare := filepath.Join(x.legacyReposDirectory(), filepath.FromSlash(repo.Name)+bareRepoSuffix)
	rel, err := filepath.Rel(oldBare, oldPath)
	if err != nil || rel == "." || !pathIsWithin(oldBare, oldPath) {
		return "", nil
	}
	path := filepath.Join(repo.BarePath, rel)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil
	}
	content, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return "", nil
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir: ")
	if !ok {
		return "", nil
	}
	oldAdmin := filepath.Join(oldBare, "worktrees")
	adminRel, err := filepath.Rel(oldAdmin, filepath.Clean(gitdir))
	if err != nil || adminRel == "." || adminRel == ".." || filepath.Base(adminRel) != adminRel {
		return "", nil
	}
	backlink, err := os.ReadFile(filepath.Join(repo.BarePath, "worktrees", adminRel, "gitdir"))
	if err != nil || filepath.Clean(strings.TrimSpace(string(backlink))) != filepath.Join(oldPath, ".git") {
		return "", nil
	}
	if _, err := os.Lstat(oldPath); !errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return path, nil
}

func worktreeGitDirIsStale(worktreePath string, barePath string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, fmt.Errorf("read worktree gitdir %q: %w", worktreePath, err)
	}
	line := strings.TrimSpace(string(content))
	gitdir, ok := strings.CutPrefix(line, "gitdir: ")
	if !ok {
		return true, nil
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktreePath, gitdir)
	}
	return !pathIsWithin(barePath, filepath.Clean(gitdir)), nil
}
