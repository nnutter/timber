package timber

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

type config struct {
	AutoMiseTrust bool `toml:"auto_mise_trust"`
}

func NewConfigCommand(runtime Runtime) *cobra.Command {
	var enable, disable bool
	command := &cobra.Command{
		Use:   "config",
		Short: "Show or change Timber settings",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			settings, err := runtime.loadConfig()
			if err != nil {
				return err
			}
			if command.Flags().Changed("auto-mise-trust") || command.Flags().Changed("no-auto-mise-trust") {
				settings.AutoMiseTrust = enable
				if command.Flags().Changed("no-auto-mise-trust") {
					settings.AutoMiseTrust = !disable
				}
				if err := writeConfig(runtime.configPath(), settings); err != nil {
					return err
				}
			}
			return toml.NewEncoder(command.OutOrStdout()).Encode(settings)
		},
	}
	command.Flags().BoolVar(&enable, "auto-mise-trust", false, "Trust mise.toml in worktrees owned by you or your GitHub organizations")
	command.Flags().BoolVar(&disable, "no-auto-mise-trust", false, "Disable automatic mise trust")
	command.MarkFlagsMutuallyExclusive("auto-mise-trust", "no-auto-mise-trust")
	return command
}

func (x Runtime) configPath() string {
	return filepath.Join(x.xdgConfigHome(), "timber", "config.toml")
}

func (x Runtime) loadConfig() (config, error) {
	path := x.configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return config{}, fmt.Errorf("create config directory: %w", err)
	}
	// Do not overwrite settings if another process creates the file first.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		encodeErr := toml.NewEncoder(file).Encode(config{})
		if err := errors.Join(encodeErr, file.Close()); err != nil {
			return config{}, fmt.Errorf("create config: %w", err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return config{}, fmt.Errorf("create config: %w", err)
	}
	var settings config
	metadata, err := toml.DecodeFile(path, &settings)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return config{}, fmt.Errorf("unknown config setting: %s", keys[0])
	}
	return settings, nil
}

func writeConfig(path string, settings config) error {
	var contents bytes.Buffer
	if err := toml.NewEncoder(&contents).Encode(settings); err != nil {
		return err
	}
	return writePrivateFile(path, contents.Bytes())
}

// Replace complete files so concurrent readers never see a partial document.
func writePrivateFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".timber-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(contents)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
