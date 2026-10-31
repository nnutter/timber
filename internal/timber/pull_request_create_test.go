package timber

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pullRequestBranch = "feature/pull"

// fakeGitHub records the requests a pull request creation makes.
type fakeGitHub struct {
	mu       sync.Mutex
	requests []recordedRequest
	handlers map[string]http.HandlerFunc
}

type recordedRequest struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	t.Helper()
	github := &fakeGitHub{
		handlers: map[string]http.HandlerFunc{
			"GET /repos/owner/timber/branches/feature/pull": branchHandler(""),
			"POST /repos/owner/timber/pulls":                createdPullRequestHandler("https://github.com/owner/timber/pull/7"),
		},
	}
	server := httptest.NewServer(http.HandlerFunc(github.serve))
	t.Cleanup(server.Close)
	return github, server
}

func (x *fakeGitHub) serve(writer http.ResponseWriter, request *http.Request) {
	recorded := recordedRequest{method: request.Method, path: request.URL.Path, auth: request.Header.Get("Authorization")}
	if request.Body != nil {
		_ = json.NewDecoder(request.Body).Decode(&recorded.body)
	}

	x.mu.Lock()
	x.requests = append(x.requests, recorded)
	handler := x.handlers[request.Method+" "+request.URL.Path]
	x.mu.Unlock()

	if handler == nil {
		http.Error(writer, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	handler(writer, request)
}

func (x *fakeGitHub) recorded() []recordedRequest {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]recordedRequest(nil), x.requests...)
}

// replaceHandler overrides one endpoint for a test.
func (x *fakeGitHub) replaceHandler(key string, handler http.HandlerFunc) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.handlers[key] = handler
}

// branchHandler answers a branch lookup with sha. An empty sha reports a
// branch GitHub does not have.
func branchHandler(sha string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		if sha == "" {
			http.Error(writer, `{"message":"Branch not found"}`, http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(writer, `{"name":"`+pullRequestBranch+`","commit":{"sha":"`+sha+`"}}`)
	}
}

func createdPullRequestHandler(url string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `{"number":7,"html_url":"`+url+`"}`)
	}
}

// preparePullRequestRepository creates a worktree whose origin claims to be a
// GitHub repository, and points the runtime at a fake GitHub API.
func preparePullRequestRepository(t *testing.T) (testRepository, Runtime, *fakeGitHub) {
	t.Helper()

	repository := newTestRepository(t)
	path, _, err := repository.runtime.CreateManagedWorktree(t.Context(), &bytes.Buffer{}, io.Discard, "", at(testRepoName, pullRequestBranch), "", false, "", false, false, testCreateReporters())
	require.NoError(t, err)
	assert.Equal(t, repository.worktreePath(pullRequestBranch), path)

	runGitCommand(t, repository.barePath, "remote", "set-url", remoteName, "git@github.com:owner/timber.git")

	github, server := newFakeGitHub(t)
	runtime := repository.runtime
	runtime.Environment = replaceTestEnvironment(runtime.Environment, "GH_TOKEN=test-token")
	runtime.GitHubAPIBaseURL = server.URL + "/"
	return repository, runtime, github
}

// useForkUpstream points the pull request branch at a second remote, as a
// GitHub fork workflow does.
func useForkUpstream(t *testing.T, repository testRepository) {
	t.Helper()

	runGitCommand(t, repository.barePath, "remote", "add", "fork", "git@github.com:contributor/timber.git")
	mainSHA := strings.TrimSpace(runGitCommand(t, repository.barePath, "rev-parse", "main"))
	runGitCommand(t, repository.barePath, "update-ref", "refs/remotes/fork/main", mainSHA)
	runGitCommand(t, repository.worktreePath(pullRequestBranch), "branch", "--set-upstream-to", "fork/main", pullRequestBranch)
}

func TestCreatePullRequestCreatesOnGitHub(t *testing.T) {
	repository, runtime, github := preparePullRequestRepository(t)
	headSHA := strings.TrimSpace(runGitCommand(t, repository.worktreePath(pullRequestBranch), "rev-parse", "HEAD"))
	github.replaceHandler("GET /repos/owner/timber/branches/feature/pull", branchHandler(headSHA))

	url, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
		Body:  "Timber can now open pull requests.",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/timber/pull/7", url)

	requests := github.recorded()
	require.Len(t, requests, 2)
	for _, request := range requests {
		assert.Equal(t, "Bearer test-token", request.auth)
	}
	assert.Equal(t, "GET /repos/owner/timber/branches/feature/pull", requests[0].method+" "+requests[0].path)

	created := requests[1]
	assert.Equal(t, "POST /repos/owner/timber/pulls", created.method+" "+created.path)
	assert.Equal(t, map[string]any{
		"title": "Add a pull request command",
		"head":  pullRequestBranch,
		"base":  "main",
		"body":  "Timber can now open pull requests.",
		"draft": false,
	}, created.body)
}

func TestCreatePullRequestRequiresPushedBranch(t *testing.T) {
	_, runtime, _ := preparePullRequestRepository(t)

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not on GitHub")
	assert.Contains(t, err.Error(), "git push origin "+pullRequestBranch)
}

func TestCreatePullRequestRejectsBranchBehindGitHub(t *testing.T) {
	repository, runtime, github := preparePullRequestRepository(t)
	github.replaceHandler("GET /repos/owner/timber/branches/feature/pull", branchHandler("0000000000000000000000000000000000000000"))

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is at 0000000 on GitHub")
	assert.Contains(t, err.Error(), strings.TrimSpace(runGitCommand(t, repository.worktreePath(pullRequestBranch), "rev-parse", "--short", "HEAD")))
}

func TestCreatePullRequestRejectsNonGitHubRemote(t *testing.T) {
	repository, runtime, _ := preparePullRequestRepository(t)
	runGitCommand(t, repository.barePath, "remote", "set-url", remoteName, "git@gitlab.com:owner/timber.git")

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `remote host "gitlab.com" is not github.com`)
}

func TestCreatePullRequestRequiresBaseWithoutRemoteUpstream(t *testing.T) {
	repository, runtime, _ := preparePullRequestRepository(t)
	runGitCommand(t, repository.worktreePath(pullRequestBranch), "branch", "--unset-upstream")

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--base")
}

func TestCreatePullRequestUsesExplicitBase(t *testing.T) {
	repository, runtime, github := preparePullRequestRepository(t)
	headSHA := strings.TrimSpace(runGitCommand(t, repository.worktreePath(pullRequestBranch), "rev-parse", "HEAD"))
	github.replaceHandler("GET /repos/owner/timber/branches/feature/pull", branchHandler(headSHA))

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
		Base:  "develop",
	})
	require.NoError(t, err)

	requests := github.recorded()
	require.Len(t, requests, 2)
	assert.Equal(t, "develop", requests[1].body["base"])
}

func TestCreatePullRequestRequiresBaseForSelfTrackingBranch(t *testing.T) {
	repository, runtime, _ := preparePullRequestRepository(t)
	// A self-named upstream names no separate base branch, and Timber must not
	// let GitHub choose one.
	runGitCommand(t, repository.worktreePath(pullRequestBranch), "config", "branch."+pullRequestBranch+".remote", remoteName)
	runGitCommand(t, repository.worktreePath(pullRequestBranch), "config", "branch."+pullRequestBranch+".merge", "refs/heads/"+pullRequestBranch)

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--base")
}

func TestCreatePullRequestResolvesRepositoryWithoutUpstream(t *testing.T) {
	repository, runtime, github := preparePullRequestRepository(t)
	headSHA := strings.TrimSpace(runGitCommand(t, repository.worktreePath(pullRequestBranch), "rev-parse", "HEAD"))
	runGitCommand(t, repository.worktreePath(pullRequestBranch), "branch", "--unset-upstream")
	github.replaceHandler("GET /repos/owner/timber/branches/"+pullRequestBranch, branchHandler(headSHA))

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
		Base:  "develop",
	})
	require.NoError(t, err)

	requests := github.recorded()
	require.Len(t, requests, 2)
	created := requests[1]
	assert.Equal(t, "POST /repos/owner/timber/pulls", created.method+" "+created.path)
	assert.Equal(t, "develop", created.body["base"])
}

func TestCreatePullRequestUsesUpstreamRemoteRepository(t *testing.T) {
	repository, runtime, github := preparePullRequestRepository(t)
	headSHA := strings.TrimSpace(runGitCommand(t, repository.worktreePath(pullRequestBranch), "rev-parse", "HEAD"))
	useForkUpstream(t, repository)
	github.replaceHandler("GET /repos/contributor/timber/branches/"+pullRequestBranch, branchHandler(headSHA))
	github.replaceHandler("POST /repos/contributor/timber/pulls", createdPullRequestHandler("https://github.com/contributor/timber/pull/9"))

	url, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/contributor/timber/pull/9", url)

	requests := github.recorded()
	require.Len(t, requests, 2)
	created := requests[1]
	assert.Equal(t, "POST /repos/contributor/timber/pulls", created.method+" "+created.path)
	assert.Equal(t, "main", created.body["base"])
}

func TestCreatePullRequestPushesToUpstreamRemote(t *testing.T) {
	repository, runtime, _ := preparePullRequestRepository(t)
	useForkUpstream(t, repository)

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw:   at(testRepoName, pullRequestBranch),
		Title: "Add a pull request command",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git push fork "+pullRequestBranch)
}

func TestCreatePullRequestRequiresTitle(t *testing.T) {
	_, runtime, _ := preparePullRequestRepository(t)

	_, err := runtime.CreatePullRequest(t.Context(), strings.NewReader(""), PullRequestDraft{
		Raw: at(testRepoName, pullRequestBranch),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs a title")
}

func TestGitHubRepositoryFromRemoteURL(t *testing.T) {
	for _, test := range []struct {
		remote string
		owner  string
		repo   string
	}{
		{remote: "git@github.com:owner/timber.git", owner: "owner", repo: "timber"},
		{remote: "https://github.com/owner/timber.git", owner: "owner", repo: "timber"},
		{remote: "https://github.com/owner/timber", owner: "owner", repo: "timber"},
		{remote: "ssh://git@github.com/owner/timber.git", owner: "owner", repo: "timber"},
	} {
		t.Run(test.remote, func(t *testing.T) {
			owner, repo, err := githubRepositoryFromRemoteURL(test.remote)
			require.NoError(t, err)
			assert.Equal(t, test.owner, owner)
			assert.Equal(t, test.repo, repo)
		})
	}

	for _, remote := range []string{
		"git@gitlab.com:owner/timber.git",
		"/tmp/remote.git",
		"git@github.com:owner/timber/extra.git",
	} {
		t.Run(remote, func(t *testing.T) {
			_, _, err := githubRepositoryFromRemoteURL(remote)
			require.Error(t, err)
		})
	}
}
