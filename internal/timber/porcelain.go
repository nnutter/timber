package timber

import "strings"

func parsePorcelainBlock(block string) (porcelainWorktree, bool) {
	if strings.TrimSpace(block) == "" {
		return porcelainWorktree{}, false
	}
	var worktree porcelainWorktree
	for line := range strings.SplitSeq(block, "\n") {
		applyPorcelainLine(&worktree, line)
	}
	if worktree.Path == "" {
		return porcelainWorktree{}, false
	}
	return worktree, true
}

func applyPorcelainLine(worktree *porcelainWorktree, line string) {
	switch {
	case strings.HasPrefix(line, "worktree "):
		worktree.Path = strings.TrimPrefix(line, "worktree ")
	case strings.HasPrefix(line, "branch "):
		worktree.BranchRef = strings.TrimPrefix(line, "branch ")
	case strings.HasPrefix(line, "HEAD "):
		worktree.CommitHash = strings.TrimPrefix(line, "HEAD ")
	case line == "detached":
		worktree.Detached = true
	case line == "bare":
		worktree.Bare = true
	case strings.HasPrefix(line, "prunable "):
		worktree.Prunable = strings.TrimPrefix(line, "prunable ")
	}
}
