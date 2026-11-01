package list

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nnutter/timber/internal/timber"
)

var (
	aheadStatusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	behindStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

type listStatusFormatter struct {
	aheadCountWidth  int
	behindCountWidth int
}

func (x listStatusFormatter) format(worktree timber.ListWorktreeView) string {
	// Merged work needs no divergence detail; name the state instead.
	if worktree.Merged {
		if worktree.Upstream != "" && worktree.Upstream != worktree.DefaultUpstream {
			return "merged [" + worktree.Upstream + "]"
		}
		return "merged"
	}
	parts := make([]string, 0, 3)
	if x.aheadCountWidth > 0 {
		indicator := formatListStatusIndicator("↑", worktree.Ahead, x.aheadCountWidth)
		if worktree.Ahead > 0 {
			indicator = aheadStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	if x.behindCountWidth > 0 {
		indicator := formatListStatusIndicator("↓", worktree.Behind, x.behindCountWidth)
		if worktree.Behind > 0 {
			indicator = behindStatusStyle.Render(indicator)
		}
		parts = append(parts, indicator)
	}
	// The default upstream (repo HEAD) is noise; only call out an
	// upstream when the worktree tracks something else.
	if worktree.Upstream != "" && worktree.Upstream != worktree.DefaultUpstream {
		parts = append(parts, "["+worktree.Upstream+"]")
	}
	return strings.TrimRight(strings.Join(parts, " "), " ")
}

func newListStatusFormatter(worktrees []timber.ListWorktreeView) listStatusFormatter {
	var formatter listStatusFormatter
	for _, worktree := range worktrees {
		if worktree.Ahead > 0 {
			formatter.aheadCountWidth = max(formatter.aheadCountWidth, len(strconv.Itoa(worktree.Ahead)))
		}
		if worktree.Behind > 0 {
			formatter.behindCountWidth = max(formatter.behindCountWidth, len(strconv.Itoa(worktree.Behind)))
		}
	}
	return formatter
}

func formatListStatusIndicator(arrow string, count int, countWidth int) string {
	if count == 0 {
		return strings.Repeat(" ", lipgloss.Width(arrow)+countWidth)
	}
	return arrow + fmt.Sprintf("%*d", countWidth, count)
}
