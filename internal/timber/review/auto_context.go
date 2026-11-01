package review

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"strings"
)

const autoContextLimit = 64 * 1024

// commitContext never consults the working tree or accepts model-supplied refs.
type commitContext struct {
	repository Repository
	base       string
	tip        string
}

func (c commitContext) ReadFile(ctx context.Context, revision, name string) (string, error) {
	sha, err := c.revision(revision)
	if err != nil {
		return "", err
	}
	if !fs.ValidPath(name) || isSame(name, ".") || strings.ContainsAny(name, "\\\x00") || sensitiveAutoPath(name) {
		return "", fmt.Errorf("file is outside automatic review context")
	}
	return c.repository.autoGit(ctx, autoContextLimit, "cat-file", "blob", sha+":"+name)
}

func (c commitContext) Search(ctx context.Context, revision, query string) (string, error) {
	sha, err := c.revision(revision)
	if err != nil {
		return "", err
	}
	if query == "" || isLess(256, len(query)) || strings.ContainsAny(query, "\x00\r\n") {
		return "", fmt.Errorf("search requires a nonempty single-line literal of at most 256 bytes")
	}
	text, err := c.repository.autoGit(ctx, autoContextLimit, "grep", "--no-color", "-n", "-I", "-F", "-e", query, sha, "--", ".")
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && isSame(exit.ExitCode(), 1) {
		return "", nil // git grep uses exit 1 for no matches.
	}
	if err != nil {
		return "", err
	}
	var filtered strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 4096), autoContextLimit)
	for scanner.Scan() {
		line := scanner.Text()
		// Drop the tree-ish prefix Git prints so the model reads path and text.
		_, rest, found := strings.Cut(line, ":")
		name, _, ok := strings.Cut(rest, ":")
		if !found || isFalse(ok) || sensitiveAutoPath(name) {
			continue
		}
		filtered.WriteString(rest)
		filtered.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read automatic review search: %w", err)
	}
	return filtered.String(), nil
}

func (c commitContext) revision(label string) (string, error) {
	switch label {
	case "base":
		return c.base, nil
	case "tip":
		return c.tip, nil
	default:
		return "", fmt.Errorf("context revision must be base or tip")
	}
}

// Obvious credential paths are never sent to a provider. This is not a secret scanner.
var sensitiveAutoPatterns = []string{
	".env*",
	".git",
	"*credential*",
	"*secret*",
	"*.pem",
	"*.key",
	"id_rsa*",
	"id_ed25519*",
	".netrc",
	".npmrc",
	".pypirc",
}

func sensitiveAutoPath(name string) bool {
	for component := range strings.SplitSeq(name, "/") {
		for _, pattern := range sensitiveAutoPatterns {
			if match, _ := path.Match(pattern, component); match {
				return true
			}
		}
	}
	return false
}

type autoOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *autoOutput) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.buffer.Len() {
		return 0, fmt.Errorf("automatic review context exceeds %d bytes", b.limit)
	}
	return b.buffer.Write(data)
}

// autoDiagnostics keeps Git's reason for a failure without letting a long
// message fail the command.
type autoDiagnostics struct {
	buffer bytes.Buffer
	limit  int
}

func (b *autoDiagnostics) String() string { return b.buffer.String() }

func (b *autoDiagnostics) Write(data []byte) (int, error) {
	if room := b.limit - b.buffer.Len(); room > 0 {
		b.buffer.Write(data[:min(len(data), room)])
	}
	return len(data), nil
}
