// Package review implements feature-branch commit review independently of a UI.
package review

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charm.land/log/v2"

	commitstack "github.com/nnutter/commitstack"

	"github.com/nnutter/timber/internal/process"
)

// Stack is a commit stack from commitstack.
type Stack = commitstack.Stack

// Snapshot holds the feature range and preserves Timber's review JSON format.
type Snapshot commitstack.Snapshot

func (s Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Branch string  `json:"branch"`
		Target string  `json:"target"`
		Base   string  `json:"base"`
		Head   string  `json:"head"`
		Stacks []Stack `json:"stacks"`
		Path   string  `json:"statePath"`
	}{
		Branch: s.Branch,
		Target: s.Target,
		Base:   s.Base,
		Head:   s.Head,
		Stacks: s.Stacks,
		Path:   s.Path,
	})
}

// Repository executes Git in the source checkout.
type Repository struct {
	Dir              string
	Environment      []string
	Logger           *log.Logger
	missingIDWarning *sync.Once
}

var quietLogger = log.New(io.Discard)

// NewRepository creates command-scoped diagnostics for a source checkout.
func NewRepository(dir string, logger *log.Logger) Repository {
	return Repository{Dir: dir, Logger: logger, missingIDWarning: new(sync.Once)}
}

// PruneCache delegates to commitstack.
func (r Repository) PruneCache(ctx context.Context) error {
	return r.commitStack().PruneCache(ctx)
}

// Resolve delegates to commitstack.
func (r Repository) Resolve(ctx context.Context, ref string) (string, error) {
	return r.commitStack().Resolve(ctx, ref)
}

// Snapshot requires an upstream and a single, linear feature-branch range.
func (r Repository) Snapshot(ctx context.Context) (Snapshot, error) {
	started := time.Now()
	defer func() { r.logger().Debug("Repository snapshot completed", "elapsed", time.Since(started)) }()
	view, err := r.commitStack().Snapshot(ctx)
	if err != nil {
		return Snapshot(view), err
	}
	warnMissingChangeIDs(r, Snapshot(view))
	return Snapshot(view), nil
}

// WarmCache delegates to commitstack.
func (r Repository) WarmCache(ctx context.Context, ref string) error {
	return r.commitStack().WarmCache(ctx, ref)
}

// autoGit bounds output in memory and disables interactive Git behavior.
func (r Repository) autoGit(ctx context.Context, limit int, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := process.Command(ctx, "git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(slices.Clone(r.environment()), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1")
	out := autoOutput{limit: limit}
	diagnostics := autoDiagnostics{limit: 4096}
	cmd.Stdout, cmd.Stderr = &out, &diagnostics
	if err := cmd.Run(); err != nil {
		if message := strings.TrimSpace(diagnostics.String()); message != "" {
			return "", fmt.Errorf("read automatic review context: %w: %s", err, message)
		}
		return "", fmt.Errorf("read automatic review context: %w", err)
	}
	if !utf8.Valid(out.buffer.Bytes()) || !isLess(bytes.IndexByte(out.buffer.Bytes(), 0), 0) {
		return "", fmt.Errorf("automatic review requires UTF-8 text context")
	}
	return out.buffer.String(), nil
}

func (r Repository) checkoutView(ctx context.Context) (Snapshot, string, error) {
	view, repo, err := r.commitStack().CheckoutView(ctx)
	return Snapshot(view), repo, err
}

func (r Repository) commitStack() commitstack.Repository {
	repo := commitstack.NewWithLogger(r.Dir, r.Environment, r.logger())
	return repo
}

func (r Repository) environment() []string {
	if r.Environment == nil {
		return os.Environ()
	}
	return r.Environment
}

func (r Repository) git(ctx context.Context, args ...string) (string, error) {
	out, err := r.gitOutput(ctx, args...)
	return strings.TrimSpace(out), err
}

func (r Repository) gitInput(ctx context.Context, input string, args ...string) (string, error) {
	started := time.Now()
	defer func() { r.logger().Debug("Git command completed", "args", args, "elapsed", time.Since(started)) }()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := process.Command(ctx, "git", args...)
	cmd.Dir = r.Dir
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(slices.Clone(r.environment()), "LC_ALL=C")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.HasPrefix(stderr.String(), "fatal: not a git repository") {
			return "", fmt.Errorf("timber review must be used from inside a Git repository")
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func (r Repository) gitOutput(ctx context.Context, args ...string) (string, error) {
	return r.gitInput(ctx, "", args...)
}

func (r Repository) logger() *log.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return quietLogger
}

// reviewBranch identifies the branch whose review lock applies during a replay.
func (r Repository) reviewBranch(ctx context.Context) (string, error) {
	branch, err := r.git(ctx, "symbolic-ref", "--quiet", "HEAD")
	if err == nil {
		return branch, nil
	}
	if !detachedHEADExit(err) {
		return "", err
	}
	for _, backend := range []string{"rebase-merge", "rebase-apply"} {
		path, pathErr := r.git(ctx, "rev-parse", "--path-format=absolute", "--git-path", backend+"/head-name")
		if pathErr != nil {
			return "", pathErr
		}
		data, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return "", readErr
		}
		branch = strings.TrimSpace(string(data))
		if !strings.HasPrefix(branch, "refs/heads/") {
			return "", fmt.Errorf("rebase does not identify a local branch")
		}
		if _, err := r.git(ctx, "check-ref-format", branch); err != nil {
			return "", err
		}
		return branch, nil
	}
	return "", err
}

// tuicr runs with the same environment as the repository's Git commands.
func (r Repository) tuicr(ctx context.Context, args ...string) (*exec.Cmd, error) {
	name := "tuicr"
	if r.Environment != nil {
		var err error
		name, err = lookPathInEnvironment(name, r.Environment)
		if err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.Environment
	return cmd, nil
}

func environmentValue(environment []string, key string) string {
	if environment == nil {
		return os.Getenv(key)
	}
	value := ""
	prefix := key + "="
	for _, entry := range environment {
		if found, ok := strings.CutPrefix(entry, prefix); ok {
			value = found
		}
	}
	return value
}

func lookPathInEnvironment(name string, environment []string) (string, error) {
	for dir := range strings.SplitSeq(environmentValue(environment, "PATH"), string(os.PathListSeparator)) {
		if dir == "" {
			dir = "."
		}
		path, err := exec.LookPath(dir + string(os.PathSeparator) + name)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(path) {
			return "", &exec.Error{Name: path, Err: exec.ErrDot}
		}
		return path, nil
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func validateCommits(commits []string) error {
	return commitstack.ValidateCommits(commits)
}

func detachedHEADExit(err error) bool {
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		return false
	}
	return exit.ExitCode() == 1
}

func warnMissingChangeIDs(r Repository, view Snapshot) {
	if r.missingIDWarning == nil {
		return
	}
	state, err := (Store{Path: view.Path}).Load()
	if err != nil {
		return
	}
	missing := 0
	for _, stack := range view.Stacks {
		if stack.ChangeID != "" {
			continue
		}
		if isApproved(state.latest(stack)) {
			continue
		}
		missing++
	}
	if missing == 0 {
		return
	}
	r.missingIDWarning.Do(func() {
		r.logger().Warn("Original commits lack Change-Id trailers; add them to improve lineage tracking after rewrites", "commits", missing)
	})
}
