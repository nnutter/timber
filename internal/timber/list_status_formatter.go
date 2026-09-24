package timber

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

type listStatusFormatter struct {
	aheadCountWidth  int
	behindCountWidth int
}

func newListStatusFormatter(worktrees []managedWorktree) listStatusFormatter {
	var formatter listStatusFormatter
	for _, worktree := range worktrees {
		status := worktree.ListStatus
		if status.Ahead > 0 {
			formatter.aheadCountWidth = max(formatter.aheadCountWidth, len(strconv.Itoa(status.Ahead)))
		}
		if status.Behind > 0 {
			formatter.behindCountWidth = max(formatter.behindCountWidth, len(strconv.Itoa(status.Behind)))
		}
	}
	return formatter
}

func (x listStatusFormatter) format(worktree managedWorktree) string {
	status := worktree.ListStatus
	// Merged work needs no divergence detail; name the state instead.
	if worktree.Merged {
		if status.Upstream != "" && status.Upstream != worktree.DefaultUpstream {
			return "merged [" + status.Upstream + "]"
		}
		return "merged"
	}
	parts := make([]string, 0, 3)
	if x.aheadCountWidth > 0 {
		indicator := formatListStatusIndicator("↑", status.Ahead, x.aheadCountWidth)
		if status.Ahead > 0 {
			indicator = aheadStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if x.behindCountWidth > 0 {
		indicator := formatListStatusIndicator("↓", status.Behind, x.behindCountWidth)
		if status.Behind > 0 {
			indicator = behindStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	// The default upstream (repo HEAD) is noise; only call out an
	// upstream when the worktree tracks something else.
	if status.Upstream != "" && status.Upstream != worktree.DefaultUpstream {
		parts = append(parts, "["+status.Upstream+"]")
	}
	return strings.TrimRight(strings.Join(parts, " "), " ")
}

func formatListStatusIndicator(arrow string, count int, countWidth int) string {
	if count == 0 {
		return strings.Repeat(" ", lipgloss.Width(arrow)+countWidth)
	}
	return arrow + fmt.Sprintf("%*d", countWidth, count)
}
