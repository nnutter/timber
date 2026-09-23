package timber

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// fallbackEditors is tried in order when EDITOR is unset.
var fallbackEditors = []string{"nvim", "nano", "vim", "vi"}

type todoCommandOptions struct {
	runtime  Runtime
	pathOnly bool
}

func NewTodoCommand(runtime Runtime) *cobra.Command {
	options := &todoCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:   "todo",
		Short: "Open the worktree-specific TODO.md",
		Args:  cobra.NoArgs,
		RunE:  options.Execute,
	}
	command.Flags().BoolVar(&options.pathOnly, "path", false, "Print the TODO.md path instead of opening it")
	return command
}

func (x *todoCommandOptions) Execute(command *cobra.Command, _ []string) error {
	gitDirResult, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	gitDir := filepath.Clean(gitDirResult.stdout)

	bareResult, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "rev-parse", "--is-bare-repository")
	if err != nil {
		return err
	}
	if bareResult.stdout == "true" {
		return fmt.Errorf("not inside a worktree")
	}

	todoPath := filepath.Join(gitDir, "TODO.md")
	file, err := os.OpenFile(todoPath, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create TODO.md: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("create TODO.md: %w", err)
	}

	if x.pathOnly {
		_, err := fmt.Fprintln(command.OutOrStdout(), todoPath)
		return err
	}

	editorLine, err := editorCommandLine(x.runtime)
	if err != nil {
		return err
	}
	fields := strings.Fields(editorLine)
	editorArgs := append(fields[1:], todoPath)

	editorCommand := x.runtime.command(command.Context(), fields[0], editorArgs...)
	editorCommand.Dir = x.runtime.CurrentDirectory
	editorCommand.Stdin = command.InOrStdin()
	editorCommand.Stdout = command.OutOrStdout()
	editorCommand.Stderr = command.ErrOrStderr()
	if err := editorCommand.Run(); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func editorCommandLine(runtime Runtime) (string, error) {
	if runtime.Environment != nil {
		for _, entry := range runtime.Environment {
			key, value, _ := strings.Cut(entry, "=")
			if key == "EDITOR" && value != "" {
				return value, nil
			}
		}
		if candidate, found := lookupFallbackEditor(runtimePathEnv(runtime.Environment)); found {
			return candidate, nil
		}
	} else {
		if editor := os.Getenv("EDITOR"); editor != "" {
			return editor, nil
		}
		for _, candidate := range fallbackEditors {
			if _, err := exec.LookPath(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("EDITOR is not set and no fallback editor found (tried %s)", strings.Join(fallbackEditors, ", "))
}

func runtimePathEnv(environment []string) string {
	for _, entry := range environment {
		if key, value, _ := strings.Cut(entry, "="); key == "PATH" {
			return value
		}
	}
	return ""
}

func lookupFallbackEditor(pathEnv string) (string, bool) {
	if pathEnv == "" {
		return "", false
	}
	for _, candidate := range fallbackEditors {
		for dir := range strings.SplitSeq(pathEnv, string(os.PathListSeparator)) {
			if dir == "" {
				continue
			}
			fullPath := filepath.Join(dir, candidate)
			info, err := os.Stat(fullPath)
			if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
				continue
			}
			return candidate, true
		}
	}
	return "", false
}
