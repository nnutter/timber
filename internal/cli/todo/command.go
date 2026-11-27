package todo

import (
	"context"
	_ "embed"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	tododata "github.com/nnutter/timber/internal/timber/todo"
)

//go:embed template.md
var todoTemplate string

// fallbackEditors is tried in order when EDITOR is unset.
var fallbackEditors = []string{"nvim", "nano", "vim", "vi"}

type Dependencies struct {
	CurrentDirectory  string
	HomeDirectory     string
	Environment       []string
	SkillContent      string
	ResolveGitDir     func(io.Reader, []string) (string, error)
	ResolveRepoGitDir func(string) (string, error)
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
	worktree     bool
	both         bool
}

func (x *todoCommandOptions) Execute(command *cobra.Command, args []string) error {
	if x.installSkill {
		return installTodoOnly(x, command, args)
	}
	paths, err := resolveTodoFiles(x, command, args)
	if err != nil {
		return err
	}

	if handled, err := reportTodoCommand(x, command, paths[0]); handled || hasError(err) {
		return err
	}

	editorLine, err := editorCommandLine(x.deps.Environment)
	if err != nil {
		return err
	}
	fields := strings.Fields(editorLine)
	editorArgs := append(fields[1:], paths...)

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
		Short:             "Open the repository or worktree TODO list",
		Args:              cobra.MaximumNArgs(1),
		RunE:              options.Execute,
		ValidArgsFunction: deps.Complete,
	}
	command.Flags().BoolVar(&options.worktree, "worktree", false, "Use the worktree TODO list instead of the repository list")
	command.Flags().BoolVar(&options.pathOnly, "path", false, "Print the TODO list path instead of opening it")
	command.Flags().BoolVar(&options.installSkill, "install-skill", false, "Install the timber-todo skill to ~/.agents/skills")
	command.Flags().BoolVarP(&options.force, "force", "f", false, "Overwrite an existing installed skill")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "Print TODO items as JSON instead of opening the file")
	command.Flags().BoolVar(&options.both, "both", false, "Open the repository and worktree TODO lists together")
	command.MarkFlagsMutuallyExclusive("path", "install-skill", "json", "both")
	command.MarkFlagsMutuallyExclusive("worktree", "install-skill", "both")
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

func resolveTodoFiles(x *todoCommandOptions, command *cobra.Command, args []string) ([]string, error) {
	worktreeDir, err := x.deps.ResolveGitDir(command.InOrStdin(), args)
	if err != nil {
		return nil, err
	}
	dirs := []string{worktreeDir}
	if !x.worktree {
		repoDir, err := x.deps.ResolveRepoGitDir(worktreeDir)
		if err != nil {
			return nil, err
		}
		dirs[0] = repoDir
		if x.both && differentGitDirs(repoDir, worktreeDir) {
			dirs = append(dirs, worktreeDir)
		}
	}
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		path, err := ensureTodoFile(dir)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func differentGitDirs(left, right string) bool { return left != right }

func ensureTodoFile(gitDir string) (string, error) {
	todoPath := tododata.Path(gitDir)
	if err := migrateRepoTodo(tododata.LegacyRepoPath(gitDir), todoPath); err != nil {
		return "", fmt.Errorf("migrate repository TODO file: %w", err)
	}
	if err := initializeTodoFile(todoPath); err != nil {
		return "", fmt.Errorf("initialize TODO file: %w", err)
	}
	return todoPath, nil
}

func initializeTodoFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if info.Size() != 0 {
		return nil
	}
	file, err = os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = file.WriteString(todoTemplate)
	return errors.Join(err, file.Close())
}

// migrateRepoTodo preserves notes from both scopes when the main worktree
// already has its own TODO.md.
func migrateRepoTodo(legacy, path string) error {
	contents, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.Rename(legacy, path)
	}
	if err != nil {
		return err
	}
	if len(contents) != 0 {
		if info.Size() != 0 {
			contents = append([]byte("\n\n"), contents...)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		_, err = file.Write(contents)
		if err := errors.Join(err, file.Close()); err != nil {
			return err
		}
	}
	return os.Remove(legacy)
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
