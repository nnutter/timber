package timber

import (
	"errors"
	"fmt"
	"io"

	"github.com/nnutter/timber/internal/terminal"
)

type repoSelection struct {
	runtime  Runtime
	RepoName string
}

// reposToConsider returns the qualifier repo if set, else every registered
// repository. It does not show the repository picker.
func (x *repoSelection) reposToConsider() ([]registeredRepo, error) {
	if x.RepoName != "" {
		repo, err := x.runtime.registeredRepoByName(x.RepoName)
		if err != nil {
			return nil, err
		}
		return []registeredRepo{repo}, nil
	}
	return x.runtime.listRegisteredRepos()
}

func (x *repoSelection) resolve(input io.Reader) (registeredRepo, *Repository, error) {
	if x.RepoName != "" {
		return x.resolveNamed(x.RepoName)
	}
	if repo, repository, err := x.tryResolveCurrent(); err == nil {
		return repo, repository, nil
	}
	return x.resolvePrompt(input)
}

func (x *repoSelection) resolveForWorktree(worktreeName string, input io.Reader) (registeredRepo, *Repository, error) {
	if x.RepoName != "" {
		return x.resolveNamed(x.RepoName)
	}
	if worktreeName != "" {
		repoName, err := x.runtime.inferUniqueRepoForWorktree(worktreeName)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		return x.resolveNamed(repoName)
	}
	if repo, repository, err := x.tryResolveCurrent(); err == nil {
		return repo, repository, nil
	}
	return x.resolvePrompt(input)
}

func (x *repoSelection) resolveNamed(name string) (registeredRepo, *Repository, error) {
	repository, repo, err := x.runtime.openRegisteredRepository(name)
	return repo, repository, err
}

func (x *repoSelection) resolvePrompt(input io.Reader) (registeredRepo, *Repository, error) {
	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}
	if len(repos) == 0 {
		return registeredRepo{}, nil, errors.New("no registered repositories; run timber repo add first")
	}

	if !terminal.IsInteractive(input) {
		return registeredRepo{}, nil, errors.New("repository selection requires @<repo>, a managed worktree cwd, or an interactive terminal")
	}

	prompter := x.runtime.RepoPrompter
	if prompter == nil {
		return registeredRepo{}, nil, errors.New("repository selection requires a repository prompter")
	}

	refs := make([]RepoRef, 0, len(repos))
	for _, repo := range repos {
		refs = append(refs, repo.ref())
	}
	selected, err := prompter.Prompt(input, refs)
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repo, err := x.runtime.registeredRepoByName(selected.Name)
	if err != nil {
		return registeredRepo{}, nil, err
	}
	repository, err := openBareRepository(x.runtime, repo.BarePath)
	if err != nil {
		return registeredRepo{}, nil, err
	}
	return repo, repository, nil
}

// resolveQualifiedWorktree resolves a [name[@repo]] selector the way the
// worktree commands accept it, returning the repository and the selected
// managed worktree.
func (x *repoSelection) resolveQualifiedWorktree(input io.Reader, raw string) (registeredRepo, *Repository, managedWorktree, error) {
	qualified, err := x.runtime.parseQualifiedName(raw)
	if err != nil {
		return registeredRepo{}, nil, managedWorktree{}, err
	}
	if qualified.Repo != "" {
		x.RepoName = qualified.Repo
	}

	repo, repository, err := x.resolveForWorktree(qualified.Name, input)
	if err != nil {
		return registeredRepo{}, nil, managedWorktree{}, err
	}
	worktrees, err := x.runtime.managedWorktreesFromRepository(repository, repo.Name)
	if err != nil {
		return registeredRepo{}, nil, managedWorktree{}, err
	}
	worktree, err := x.runtime.selectManagedWorktree(worktrees, qualified.Name)
	if err != nil {
		return registeredRepo{}, nil, managedWorktree{}, err
	}
	return repo, repository, worktree, nil
}

func (x *repoSelection) tryResolveCurrent() (registeredRepo, *Repository, error) {
	currentDirectory := x.runtime.CurrentDirectory
	worktreeRepository, err := openRepository(x.runtime, currentDirectory)
	if err != nil {
		return registeredRepo{}, nil, fmt.Errorf("current directory is not inside a Git worktree: %w", err)
	}

	commonDir, err := worktreeRepository.commonGitDir()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	repos, err := x.runtime.listRegisteredRepos()
	if err != nil {
		return registeredRepo{}, nil, err
	}

	for _, repo := range repos {
		same, err := samePath(repo.BarePath, commonDir)
		if err != nil {
			return registeredRepo{}, nil, err
		}
		if same {
			repository, err := openBareRepository(x.runtime, repo.BarePath)
			if err != nil {
				return registeredRepo{}, nil, err
			}
			return repo, repository, nil
		}
	}

	return registeredRepo{}, nil, fmt.Errorf(
		"current worktree is not part of a registered repository (common git dir %s)",
		commonDir,
	)
}

func samePath(left string, right string) (bool, error) {
	return canonicalPath(left) == canonicalPath(right), nil
}
