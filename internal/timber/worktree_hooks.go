package timber

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
