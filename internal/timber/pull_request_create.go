package timber

import (
	"context"
	"fmt"
	"io"
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

	remote, err := repository.pullRequestRemote(head)
	if err != nil {
		return pullRequestTarget{}, err
	}
	remoteURL, configured, err := repository.gitConfigValue("remote." + remote + ".url")
	if err != nil {
		return pullRequestTarget{}, err
	}
	if !configured || remoteURL == "" {
		return pullRequestTarget{}, fmt.Errorf("repository %s has no %s remote; a pull request needs a GitHub remote", worktree.Repo, remote)
	}
	owner, repo, err := githubRepositoryFromRemoteURL(remoteURL)
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

// pullRequestRemote names the remote that holds the branch's upstream, and so
// the repository the pull request belongs to. A branch with no upstream remote
// falls back to origin, which keeps --base usable on its own.
func (x *Repository) pullRequestRemote(branchName string) (string, error) {
	remote, configured, err := x.gitConfigValue("branch." + branchName + ".remote")
	if err != nil {
		return "", err
	}
	// A "." remote names a local branch, which holds no repository.
	if !configured || remote == "" || remote == "." {
		return remoteName, nil
	}
	return remote, nil
}

// pullRequestBase derives the merge base from the branch's upstream, so every
// request names a base branch and GitHub never picks one.
func (x *Repository) pullRequestBase(branchName string, remote string) (string, error) {
	upstreamRef, err := x.upstreamReference(branchName)
	if err != nil {
		return "", fmt.Errorf("branch %q needs an upstream or --base to name the pull request base: %w", branchName, err)
	}
	remoteBranch, found := strings.CutPrefix(string(upstreamRef), remoteRefPrefix+remote+"/")
	if !found {
		return "", fmt.Errorf("branch %q tracks %s, which is not a branch on %s; use --base to name the base branch", branchName, shortReference(upstreamRef), remote)
	}
	if remoteBranch == branchName {
		return "", fmt.Errorf("branch %q tracks %s/%s, so it needs --base to name the branch it merges into", branchName, remote, remoteBranch)
	}
	return remoteBranch, nil
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

func (target pullRequestTarget) pushError(problem string) error {
	return fmt.Errorf("branch %q %s but the worktree is at %s; push it first: git push %s %s",
		target.head, problem, shortCommit(target.headSHA), target.remote, target.head)
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
