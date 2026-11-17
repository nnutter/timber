package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/cli/complete"
	coreconfig "github.com/nnutter/timber/internal/timber/config"
)

// Store loads and saves Timber settings.
type Store interface {
	LoadConfig() (coreconfig.Settings, error)
	ReadConfig() (coreconfig.Settings, error)
	SaveConfig(coreconfig.Settings) error
}

// New constructs the config command and its subcommands.
func New(store Store) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Show or change Timber settings",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		showCommand(store),
		getCommand(store),
		setCommand(store),
		appendCommand(store),
		removeCommand(store),
	)
	return command
}

func showCommand(store Store) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show all settings",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			settings, err := store.LoadConfig()
			if err != nil {
				return err
			}
			return toml.NewEncoder(command.OutOrStdout()).Encode(settings)
		},
	}
}

func getCommand(store Store) *cobra.Command {
	return &cobra.Command{
		Use:               "get <setting>",
		Short:             "Print a setting's value",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: complete.ConfigArgs(store, complete.ConfigValueNone),
		RunE: func(command *cobra.Command, args []string) error {
			settings, err := store.ReadConfig()
			if err != nil {
				return err
			}
			values, err := coreconfig.Get(settings, args[0])
			if err != nil {
				return err
			}
			for _, value := range values {
				if _, err := fmt.Fprintln(command.OutOrStdout(), value); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func setCommand(store Store) *cobra.Command {
	return &cobra.Command{
		Use:               "set <setting> <value>",
		Short:             "Set a setting's value",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: complete.ConfigArgs(store, complete.ConfigValueSet),
		RunE: func(_ *cobra.Command, args []string) error {
			return changeSettings(store, func(settings *coreconfig.Settings) error {
				return coreconfig.Set(settings, args[0], args[1:])
			})
		},
	}
}

func appendCommand(store Store) *cobra.Command {
	return &cobra.Command{
		Use:               "append <setting> <value>...",
		Short:             "Append values to a list setting",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: complete.ConfigArgs(store, complete.ConfigValueNone),
		RunE: func(_ *cobra.Command, args []string) error {
			return changeSettings(store, func(settings *coreconfig.Settings) error {
				return coreconfig.Append(settings, args[0], args[1:])
			})
		},
	}
}

func removeCommand(store Store) *cobra.Command {
	return &cobra.Command{
		Use:               "remove <setting> [value...]",
		Aliases:           []string{"rm"},
		Short:             "Reset a setting or remove list values",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: complete.ConfigArgs(store, complete.ConfigValueRemove),
		RunE: func(_ *cobra.Command, args []string) error {
			return changeSettings(store, func(settings *coreconfig.Settings) error {
				return coreconfig.Remove(settings, args[0], args[1:])
			})
		},
	}
}

// changeSettings loads the settings, applies change, and saves the result.
func changeSettings(store Store, change func(*coreconfig.Settings) error) error {
	settings, err := store.LoadConfig()
	if err != nil {
		return err
	}
	if err := change(&settings); err != nil {
		return err
	}
	return store.SaveConfig(settings)
}
