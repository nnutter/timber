package timber

import (
	"fmt"
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
