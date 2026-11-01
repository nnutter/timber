package timber

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v91/github"
)

// PullRequestDraft holds the values one pull request creation provides.
// It is a plain description so callers do not need Timber command types.
type PullRequestDraft struct {
	// Raw is the [name[@repo]] worktree selector, empty for the current one.
	Raw string
	// Title is required; GitHub rejects a pull request without one.
	Title string
	Body  string
	// Base overrides the branch the pull request merges into. An empty value
	// uses the branch's upstream, which every request must name.
	Base  string
	Draft bool
}

// pullRequestTarget identifies the repository, branch, and commit a pull
// request would publish.
type pullRequestTarget struct {
	// remote is the remote that holds the head branch and the base repository.
	remote  string
	owner   string
	repo    string
	head    string
	base    string
	headSHA string
}

func (target pullRequestTarget) pushError(problem string) error {
	return fmt.Errorf("branch %q %s but the worktree is at %s; push it first: git push %s %s",
		target.head, problem, shortCommit(target.headSHA), target.remote, target.head)
}

// requirePushed fails unless GitHub already has the worktree's commit on the
// branch, so a pull request never advertises commits GitHub cannot see.
func (target pullRequestTarget) requirePushed(ctx context.Context, client *github.Client) error {
	branch, response, err := client.Repositories.GetBranch(ctx, target.owner, target.repo, target.head, 0)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return target.pushError("is not on GitHub")
		}
		return fmt.Errorf("read branch %q from GitHub: %w", target.head, err)
	}
	if sha := branch.GetCommit().GetSHA(); sha != target.headSHA {
		return target.pushError("is at " + shortCommit(sha) + " on GitHub")
	}
	return nil
}

// localHeadCommit reads the commit the worktree has checked out.
func localHeadCommit(runtime Runtime, worktreePath string) (string, error) {
	result, err := gitOutput(runtime, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("read worktree commit: %w", err)
	}
	return result.stdout, nil
}

func shortCommit(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

// githubRepositoryFromRemoteURL reads the owner and repository from a
// github.com remote URL and rejects every other host.
func githubRepositoryFromRemoteURL(remoteURL string) (owner, repo string, err error) {
	host, repoPath, ok := remoteHostAndPath(remoteURL)
	if !ok {
		return "", "", fmt.Errorf("remote %q is not a GitHub remote; Timber creates pull requests on %s only", remoteURL, githubHost)
	}
	if !strings.EqualFold(host, githubHost) {
		return "", "", fmt.Errorf("remote host %q is not %s; Timber creates pull requests on %s only", host, githubHost, githubHost)
	}
	repoPath = strings.TrimSuffix(strings.Trim(repoPath, "/"), bareRepoSuffix)
	owner, repo, found := strings.Cut(repoPath, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("remote %q does not name a repository as %s/<owner>/<repository>", remoteURL, githubHost)
	}
	return owner, repo, nil
}
