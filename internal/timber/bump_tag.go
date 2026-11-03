package timber

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nnutter/timber/internal/timber/bump"
)

// TagBump describes one version tag bump.
type TagBump struct {
	// Raw is the [name[@repo]] worktree selector, empty for the current one.
	Raw string
	// Kind is the version component to increment.
	Kind bump.Kind
	// Pattern limits candidate tags the way `git tag --list <pattern>` does.
	// An empty pattern considers every tag.
	Pattern string
	// NoPush creates the tag locally and leaves it unpushed.
	NoPush bool
	// Release creates a draft GitHub release for the new tag.
	Release bool
	// NoOpen keeps the browser closed after Timber reports the draft URL.
	NoOpen bool
}

// BumpResult reports what one version bump produced.
type BumpResult struct {
	// Tag is the new tag.
	Tag string
	// ReleaseURL is the draft release URL, empty without request.Release.
	ReleaseURL string
}

// refreshBeforeBump fetches the tags and branch tips that the bump depends on,
// then refuses to tag a commit that its upstream has moved past.
//
// Without the fetch, a tag created on another machine is missing from the
// candidates, so the bump repeats or precedes a version that already exists.
// Without the comparison, the tag points at a commit that origin no longer has
// at the tip, which is not the commit a release should name.
func refreshBeforeBump(runtime Runtime, directory, branch string) error {
	upstream, err := upstreamReference(runtime, directory)
	if err != nil {
		return err
	}
	for _, remote := range remotesToRefresh(upstream) {
		if !hasRemote(runtime, directory, remote) {
			continue
		}
		if _, err := gitOutput(runtime, directory, "fetch", "--tags", remote); err != nil {
			return fmt.Errorf("fetch %s: %w", remote, err)
		}
	}
	if upstream == "" {
		return nil
	}
	behind, err := commitsBehind(runtime, directory, upstream)
	if err != nil {
		return err
	}
	if behind == 0 {
		return nil
	}
	return fmt.Errorf(
		"branch %q is %s behind %s; update the worktree before bumping so the tag points at the latest commit",
		branch, commitCount(behind), upstream,
	)
}

// upstreamReference returns the branch that HEAD tracks, or "" when HEAD has
// no upstream branch or does not point at a branch.
func upstreamReference(runtime Runtime, directory string) (string, error) {
	result, err := gitOutput(runtime, directory, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", nil
	}
	return result.stdout, nil
}

// remotesToRefresh lists the remotes whose refs matter: origin, which receives
// the tag, and the remote of the tracked branch when it is a different one.
func remotesToRefresh(upstream string) []string {
	remotes := []string{"origin"}
	if remote := upstreamRemote(upstream); isAnotherRemote(remote) {
		remotes = append(remotes, remote)
	}
	return remotes
}

// upstreamRemote returns the remote part of an upstream reference, or "" when
// the upstream is a local branch.
func upstreamRemote(upstream string) string {
	remote, _, found := strings.Cut(upstream, "/")
	if !found {
		return ""
	}
	return remote
}

// isAnotherRemote reports whether remote is a remote other than origin.
func isAnotherRemote(remote string) bool {
	if remote == "" {
		return false
	}
	return remote != "origin"
}

// commitsBehind counts the upstream commits that HEAD does not have.
func commitsBehind(runtime Runtime, directory, upstream string) (int, error) {
	result, err := gitOutput(runtime, directory, "rev-list", "--count", "HEAD.."+upstream)
	if err != nil {
		return 0, fmt.Errorf("compare HEAD with %s: %w", upstream, err)
	}
	count, err := strconv.Atoi(result.stdout)
	if err != nil {
		return 0, fmt.Errorf("parse behind count for %s: %w", upstream, err)
	}
	return count, nil
}

// commitCount renders a count with the noun that matches it.
func commitCount(count int) string {
	if count == 1 {
		return "1 commit"
	}
	return strconv.Itoa(count) + " commits"
}

// hasRemote reports whether the repository configures the named remote.
func hasRemote(runtime Runtime, directory, name string) bool {
	result, err := gitOutput(runtime, directory, "remote")
	if err != nil {
		return false
	}
	for remote := range strings.SplitSeq(result.stdout, "\n") {
		if isSame(remote, name) {
			return true
		}
	}
	return false
}

// createBumpedTag creates the next tag on HEAD and pushes it unless the
// request keeps it local.
func createBumpedTag(runtime Runtime, directory string, request TagBump) (string, error) {
	latest, err := latestVersionTag(runtime, directory, request.Pattern)
	if err != nil {
		return "", err
	}
	tag, err := bump.Tag(latest, request.Kind)
	if err != nil {
		return "", err
	}
	if err := createVersionTag(runtime, directory, tag); err != nil {
		return "", err
	}
	if !request.NoPush {
		if err := pushVersionTag(runtime, directory, tag); err != nil {
			return "", err
		}
	}
	return tag, nil
}

// reportBrowserWarning tells the user the browser stayed closed without
// failing the bump.
func reportBrowserWarning(output io.Writer, err error) error {
	if output == nil {
		return nil
	}
	if _, writeErr := fmt.Fprintf(output, "warning: could not open browser: %v\n", err); writeErr != nil {
		return writeErr
	}
	return nil
}

// latestVersionTag returns the highest strict semantic version tag that
// pattern selects. Without a pattern, a repository without a usable tag
// starts from v0.0.0; with a pattern, no matching tag is an error so a typo
// never produces a fresh v0.0.1.
func latestVersionTag(runtime Runtime, directory string, pattern string) (string, error) {
	names, err := listVersionTags(runtime, directory, pattern)
	if err != nil {
		return "", err
	}
	latest, found := bump.Highest(names)
	if found {
		return latest, nil
	}
	if pattern != "" {
		return "", fmt.Errorf("no tags match pattern %q", pattern)
	}
	return bump.Version{}.String(), nil
}

// listVersionTags returns the tag names that pattern selects.
func listVersionTags(runtime Runtime, directory string, pattern string) ([]string, error) {
	args := []string{"tag", "--list"}
	if pattern != "" {
		args = append(args, pattern)
	}
	result, err := gitOutput(runtime, directory, args...)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	// Git tag names cannot contain whitespace.
	return strings.Fields(result.stdout), nil
}

// createVersionTag creates an annotated tag on HEAD whose message is the tag.
func createVersionTag(runtime Runtime, directory string, tag string) error {
	if _, err := bump.Parse(tag); err != nil {
		return err
	}
	if _, err := gitOutput(runtime, directory, "tag", "--annotate", tag, "--message", tag); err != nil {
		return fmt.Errorf("create tag %q: %w", tag, err)
	}
	return nil
}

// pushVersionTag pushes one tag to origin with the git CLI so SSH agent,
// SSH configuration, and credential helpers behave like a manual push.
func pushVersionTag(runtime Runtime, directory string, tag string) error {
	ref := "refs/tags/" + tag + ":refs/tags/" + tag
	if _, err := gitOutput(runtime, directory, "push", "origin", ref); err != nil {
		return fmt.Errorf("push tag %q: %w", tag, err)
	}
	return nil
}

// createDraftRelease creates a draft GitHub release for tag with the gh CLI
// so authentication behaves like a manual release, and returns its URL.
func createDraftRelease(ctx context.Context, runtime Runtime, directory string, tag string) (string, error) {
	if _, err := bump.Parse(tag); err != nil {
		return "", err
	}
	command := runtime.command(ctx, "gh", "release", "create", tag, "--draft", "--title", tag, "--generate-notes")
	command.Dir = directory
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("create draft release for %q: %w: %s", tag, err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	url := strings.TrimSpace(lines[len(lines)-1])
	if url == "" {
		return "", fmt.Errorf("create draft release for %q: no URL in output", tag)
	}
	return url, nil
}

// openURL opens url in the browser. Without an opener, opening is a quiet
// no-op so headless hosts still bump tags.
func openURL(ctx context.Context, runtime Runtime, url string) error {
	opener := browserOpener(runtime)
	if opener == "" {
		return nil
	}
	output, err := runtime.command(ctx, opener, url).CombinedOutput()
	if err != nil {
		return fmt.Errorf("open %q: %w: %s", url, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// browserOpener returns the browser opener on the runtime PATH, preferring
// open (macOS) and falling back to xdg-open (Linux).
func browserOpener(runtime Runtime) string {
	for _, name := range []string{"open", "xdg-open"} {
		if path := lookPath(runtime, name); path != "" {
			return path
		}
	}
	return ""
}

// lookPath searches the runtime PATH for an executable file. The runtime
// environment decides the search, not the process that runs Timber.
func lookPath(runtime Runtime, name string) string {
	path := ""
	for _, entry := range runtime.Environment {
		if value, found := strings.CutPrefix(entry, "PATH="); found {
			path = value
		}
	}
	if path == "" {
		// Without a runtime PATH, defer to the process environment.
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved
		}
		return ""
	}
	for directory := range strings.SplitSeq(path, string(os.PathListSeparator)) {
		candidate := filepath.Join(directory, name)
		info, err := os.Stat(candidate)
		if err == nil && isExecutableFile(info) {
			return candidate
		}
	}
	return ""
}

// isExecutableFile reports whether info describes a file that can run.
func isExecutableFile(info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
