package todo

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	tododata "github.com/nnutter/timber/internal/timber/todo"
)

// fallbackEditors is tried in order when EDITOR is unset.
var fallbackEditors = []string{"nvim", "nano", "vim", "vi"}

type Dependencies struct {
	CurrentDirectory  string
	HomeDirectory     string
	Environment       []string
	SkillContent      string
	ResolveGitDir     func(io.Reader, []string) (string, error)
	ResolveRepoGitDir func(io.Reader, []string) (string, error)
	Command           func(context.Context, string, ...string) *exec.Cmd
	Complete          func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	RenderStatus      func(string) string
}

type todoCommandOptions struct {
	deps         Dependencies
	pathOnly     bool
	installSkill bool
	force        bool
	jsonOutput   bool
	repo         bool
}

func (x *todoCommandOptions) Execute(command *cobra.Command, args []string) error {
	if x.installSkill {
		return installTodoOnly(x, command, args)
	}
	todoPath, err := ensureTodoFile(x, command, args)
	if err != nil {
		return err
	}

	if handled, err := reportTodoCommand(x, command, todoPath); handled || hasError(err) {
		return err
	}

	editorLine, err := editorCommandLine(x.deps.Environment)
	if err != nil {
		return err
	}
	fields := strings.Fields(editorLine)
	editorArgs := append(fields[1:], todoPath)

	editorCommand := x.deps.Command(command.Context(), fields[0], editorArgs...)
	editorCommand.Dir = x.deps.CurrentDirectory
	editorCommand.Stdin = command.InOrStdin()
	editorCommand.Stdout = command.OutOrStdout()
	editorCommand.Stderr = command.ErrOrStderr()
	if err := editorCommand.Run(); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

func (x *todoCommandOptions) installTodoSkill(command *cobra.Command) error {
	if x.deps.SkillContent == "" {
		return fmt.Errorf("timber-todo skill content is not available")
	}

	destination := filepath.Join(x.deps.HomeDirectory, ".agents", "skills", "timber-todo", "SKILL.md")
	if _, err := os.Stat(destination); err == nil && isFalse(x.force) {
		return fmt.Errorf("skill already exists at %s (use --force to overwrite)", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect skill destination %q: %w", destination, err)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(destination, []byte(x.deps.SkillContent), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	_, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", x.deps.RenderStatus("installed timber-todo skill to "+destination))
	return err
}

func (x *todoCommandOptions) reportTodoJSON(command *cobra.Command, path string) error {
	output, err := readTodoJSON(path)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode TODO.md as JSON: %w", err)
	}
	_, err = fmt.Fprintln(command.OutOrStdout(), string(encoded))
	return err
}

func New(deps Dependencies) *cobra.Command {
	options := &todoCommandOptions{deps: deps}

	command := &cobra.Command{
		Use:               "todo [name[@repo]]",
		Short:             "Open the worktree or repository TODO list",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVar(&options.repo, "repo", false, "Use the repository TODO list instead of the worktree list")
	command.Flags().BoolVar(&options.pathOnly, "path", false, "Print the TODO list path instead of opening it")
	command.Flags().BoolVar(&options.installSkill, "install-skill", false, "Install the timber-todo skill to ~/.agents/skills")
	command.Flags().BoolVarP(&options.force, "force", "f", false, "Overwrite an existing installed skill")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "Print TODO items as JSON instead of opening the file")
	command.MarkFlagsMutuallyExclusive("path", "install-skill", "json")
	command.MarkFlagsMutuallyExclusive("repo", "install-skill")
	return command
}

func editorCommandLine(environment []string) (string, error) {
	if environment != nil {
		for _, entry := range environment {
			key, value, _ := strings.Cut(entry, "=")
			if key == "EDITOR" && !isEmpty(value) {
				return value, nil
			}
		}
		if candidate, found := lookupFallbackEditor(runtimePathEnv(environment)); found {
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
			if err != nil || info.IsDir() || isNotExecutable(info) {
				continue
			}
			return candidate, true
		}
	}
	return "", false
}

func installTodoOnly(x *todoCommandOptions, command *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("install-skill takes no arguments")
	}
	return x.installTodoSkill(command)
}

func ensureTodoFile(x *todoCommandOptions, command *cobra.Command, args []string) (string, error) {
	resolveGitDir := x.deps.ResolveGitDir
	if x.repo {
		resolveGitDir = x.deps.ResolveRepoGitDir
	}
	gitDir, err := resolveGitDir(command.InOrStdin(), args)
	if err != nil {
		return "", err
	}
	todoPath := tododata.Path(gitDir)
	if x.repo {
		todoPath = filepath.Join(gitDir, "TODO.repo.md")
	}
	file, err := os.OpenFile(todoPath, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("create TODO file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("create TODO file: %w", err)
	}
	return todoPath, nil
}

func reportTodoCommand(x *todoCommandOptions, command *cobra.Command, todoPath string) (bool, error) {
	if x.pathOnly {
		_, err := fmt.Fprintln(command.OutOrStdout(), todoPath)
		return true, err
	}
	if x.jsonOutput {
		return true, x.reportTodoJSON(command, todoPath)
	}
	return false, nil
}
