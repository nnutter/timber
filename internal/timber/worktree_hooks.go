package timber

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConfigureWorktreeHooks moves one worktree to its own Git hooks directory so
// that every worktree can carry different hooks. It copies the hooks the
// worktree had before the split.
//
// Timber sets core.hooksPath only when no scope sets it already. A hooks path
// from the person's environment, a hook manager such as hk, or an earlier
// Timber run therefore stays in control.
func (runtime Runtime) ConfigureWorktreeHooks(worktreePath string) error {
	configured, err := gitSettingIsSet(runtime, worktreePath, "core.hooksPath")
	if err != nil {
		return err
	}
	if configured {
		return nil
	}

	gitDir, err := runtime.worktreeGitDir(worktreePath)
	if err != nil {
		return err
	}
	commonDirResult, err := gitOutput(runtime, worktreePath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("resolve common Git directory for %q: %w", worktreePath, err)
	}
	commonDir := filepath.Clean(commonDirResult.stdout)

	if err := runtime.enableWorktreeConfig(commonDir); err != nil {
		return err
	}

	hooksPath := filepath.Join(gitDir, "hooks")
	if err := cloneHooksDirectory(filepath.Join(commonDir, "hooks"), hooksPath); err != nil {
		return err
	}
	if _, err := gitOutput(runtime, worktreePath, "config", "--worktree", "core.hooksPath", hooksPath); err != nil {
		return fmt.Errorf("set per-worktree hooks path: %w", err)
	}
	return nil
}

// enableWorktreeConfig turns on per-worktree configuration for a repository.
//
// Git stops special-casing core.bare once this extension is on, so a bare
// repository must first record the bare marker in its main worktree
// configuration and drop it from the shared configuration. Git ignores
// config.worktree while the extension is off, so the two writes leave the
// repository usable at every step.
func (runtime Runtime) enableWorktreeConfig(commonDir string) error {
	enabled, err := gitBooleanSetting(runtime, commonDir, "extensions.worktreeConfig")
	if err != nil {
		return err
	}
	if enabled {
		return nil
	}

	bare, err := gitBooleanSetting(runtime, commonDir, "core.bare")
	if err != nil {
		return err
	}
	if bare {
		mainWorktreeConfig := filepath.Join(commonDir, "config.worktree")
		if _, err := gitOutput(runtime, commonDir, "config", "--file", mainWorktreeConfig, "core.bare", "true"); err != nil {
			return fmt.Errorf("record bare repository marker: %w", err)
		}
		if _, err := gitOutput(runtime, commonDir, "config", "--unset", "core.bare"); err != nil {
			return fmt.Errorf("remove shared bare repository marker: %w", err)
		}
	}

	if _, err := gitOutput(runtime, commonDir, "config", "extensions.worktreeConfig", "true"); err != nil {
		return fmt.Errorf("enable per-worktree configuration: %w", err)
	}
	return nil
}

// cloneHooksDirectory copies hooks into a worktree hooks directory. The copy
// keeps the hooks the worktree used before it had its own directory.
func cloneHooksDirectory(source, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create worktree hooks directory %q: %w", destination, err)
	}
	entries, err := os.ReadDir(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read shared hooks directory %q: %w", source, err)
	}
	if len(entries) == 0 {
		return nil
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return fmt.Errorf("copy shared hooks into %q: %w", destination, err)
	}
	return nil
}

// gitSettingIsSet reports whether any configuration scope sets a key.
func gitSettingIsSet(runtime Runtime, directory, key string) (bool, error) {
	_, ok, err := readGitSetting(runtime, directory, key, false)
	return ok, err
}

// gitBooleanSetting reads a Git boolean and reports whether it is true.
func gitBooleanSetting(runtime Runtime, directory, key string) (bool, error) {
	value, ok, err := readGitSetting(runtime, directory, key, true)
	if err != nil || !ok {
		return false, err
	}
	return strings.EqualFold(value, "true"), nil
}

// readGitSetting reads the last value of a setting. ok is false when no scope
// sets the setting.
func readGitSetting(runtime Runtime, directory, key string, asBoolean bool) (string, bool, error) {
	args := []string{"config", "--get", key}
	if asBoolean {
		args = []string{"config", "--get", "--bool", key}
	}
	result, err := gitOutput(runtime, directory, args...)
	if err == nil {
		return result.stdout, true, nil
	}
	if exitError, isExitError := errors.AsType[*exec.ExitError](err); isExitError && exitError.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read Git setting %s: %w", key, err)
}
