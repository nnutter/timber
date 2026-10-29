package importcmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/nnutter/timber/internal/timber"
)

func newReporter(output io.Writer, deps Dependencies) timber.ImportReporter {
	return timber.ImportReporter{
		Registered: func(repoName, barePath string) error {
			message := fmt.Sprintf("registered repository %s at %s", repoName, displayHomePath(deps.HomeDirectory, barePath))
			_, err := fmt.Fprintf(output, "%s\n", deps.RenderStatus(message))
			return err
		},
		Summary: func(summary timber.ImportSummary) error {
			for _, skip := range summary.Skipped {
				message := fmt.Sprintf("skipped worktree %s: %s", displayHomePath(deps.HomeDirectory, skip.Path), skip.Reason)
				if _, err := fmt.Fprintf(output, "%s\n", deps.RenderWarning(message)); err != nil {
					return err
				}
			}
			message := fmt.Sprintf("imported %d worktrees:", len(summary.Worktrees))
			if _, err := fmt.Fprintf(output, "%s\n", deps.RenderStatus(message)); err != nil {
				return err
			}
			for _, worktree := range summary.Worktrees {
				message := fmt.Sprintf("  %s: %s -> %s", worktree.Name,
					displayHomePath(deps.HomeDirectory, worktree.SourcePath),
					displayHomePath(deps.HomeDirectory, worktree.TargetPath))
				if _, err := fmt.Fprintf(output, "%s\n", deps.RenderStatus(message)); err != nil {
					return err
				}
			}
			if len(summary.Worktrees) > 0 {
				note := "old worktrees were moved to the system trash; restore them from there if needed"
				if _, err := fmt.Fprintf(output, "%s\n", deps.RenderStatus(note)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func displayHomePath(home, path string) string {
	if home == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "~"
	}
	if relative, ok := strings.CutPrefix(cleanPath, cleanHome+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + relative
	}
	return path
}
