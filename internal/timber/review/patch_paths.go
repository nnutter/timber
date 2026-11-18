package review

import (
	"context"
	"fmt"
	"strings"
)

func patchPaths(ctx context.Context, r Repository, base, tip string) ([]string, error) {
	out, err := r.gitOutput(ctx, "diff-tree", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z", base, tip)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
}

func batchPatchPaths(ctx context.Context, r Repository, stacks []Stack) (map[string][]string, error) {
	var input strings.Builder
	for _, stack := range stacks {
		input.WriteString(stack.Tip)
		input.WriteByte(' ')
		input.WriteString(stack.Base)
		input.WriteByte('\n')
	}
	// On stdin, the second commit supplies the parent for the first.
	// Raw records separate paths from commit headers, even when a path
	// looks like a commit ID. --always includes headers for empty diffs.
	out, err := r.gitInput(ctx, input.String(), "diff-tree", "--stdin", "--raw", "--no-renames", "-r", "-z", "--always")
	if err != nil {
		return nil, err
	}
	return parseBatchPatchPaths(out, stacks)
}

func parseBatchPatchPaths(out string, stacks []Stack) (map[string][]string, error) {
	pathsByTip := make(map[string][]string, len(stacks))
	for _, stack := range stacks {
		tip, records, ok := strings.Cut(out, "\x00")
		if !ok || !isSame(tip, stack.Tip) {
			return nil, fmt.Errorf("missing Git diff-tree header for %s", stack.Tip)
		}
		paths, rest, err := takeBatchPatchPaths(records)
		if err != nil {
			return nil, err
		}
		pathsByTip[tip] = paths
		out = rest
	}
	if out != "" {
		return nil, fmt.Errorf("unexpected trailing Git diff-tree records")
	}
	return pathsByTip, nil
}

func takeBatchPatchPaths(out string) ([]string, string, error) {
	var paths []string
	for strings.HasPrefix(out, ":") {
		_, afterHeader, _ := strings.Cut(out, "\x00")
		path, rest, ok := strings.Cut(afterHeader, "\x00")
		if !ok || isEmpty(path) {
			return nil, "", fmt.Errorf("invalid Git diff-tree path record")
		}
		paths = append(paths, path)
		out = rest
	}
	return paths, out, nil
}
