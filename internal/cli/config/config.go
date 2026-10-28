package config

import (
	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	coreconfig "github.com/nnutter/timber/internal/timber/config"
)

// Store loads and saves Timber settings.
type Store interface {
	LoadConfig() (coreconfig.Settings, error)
	SaveConfig(coreconfig.Settings) error
}

func New(store Store) *cobra.Command {
	var enable, disable, enableHK, disableHK bool
	command := &cobra.Command{
		Use:   "config",
		Short: "Show or change Timber settings",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			settings, err := store.LoadConfig()
			if err != nil {
				return err
			}
			changed := false
			if command.Flags().Changed("auto-mise-trust") || command.Flags().Changed("no-auto-mise-trust") {
				settings.AutoMiseTrust = enable
				if command.Flags().Changed("no-auto-mise-trust") {
					settings.AutoMiseTrust = !disable
				}
				settings.AutoMiseTrustDecided = true
				changed = true
			}
			if command.Flags().Changed("auto-hk-install") || command.Flags().Changed("no-auto-hk-install") {
				settings.AutoHKInstall = enableHK
				if command.Flags().Changed("no-auto-hk-install") {
					settings.AutoHKInstall = !disableHK
				}
				settings.AutoHKInstallDecided = true
				changed = true
			}
			if changed {
				if err := store.SaveConfig(settings); err != nil {
					return err
				}
			}
			return toml.NewEncoder(command.OutOrStdout()).Encode(settings)
		},
	}
	command.Flags().BoolVar(&enable, "auto-mise-trust", false, "Trust mise.toml in worktrees owned by you or your GitHub organizations")
	command.Flags().BoolVar(&disable, "no-auto-mise-trust", false, "Disable automatic mise trust")
	command.MarkFlagsMutuallyExclusive("auto-mise-trust", "no-auto-mise-trust")
	command.Flags().BoolVar(&enableHK, "auto-hk-install", false, "Install hk hooks in worktrees owned by you or your GitHub organizations")
	command.Flags().BoolVar(&disableHK, "no-auto-hk-install", false, "Disable automatic hk hook installation")
	command.MarkFlagsMutuallyExclusive("auto-hk-install", "no-auto-hk-install")
	return command
}
