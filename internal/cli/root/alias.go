package root

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/timber/config"
)

// ConfigureAliases adds the shell wrapper's lookup command and returns an
// argument expander for normal CLI invocations and delegated completion.
func ConfigureAliases(root *cobra.Command, load func() (config.Settings, error)) func([]string) ([]string, error) {
	lookup := func(name string) ([]string, error) {
		return resolveAlias(root, load, name)
	}
	root.AddCommand(&cobra.Command{
		Use:    "__expand-alias <alias>",
		Short:  "Look up a configured alias for the shell wrapper",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			target, err := lookup(args[0])
			if err != nil {
				return err
			}
			if len(target) == 0 {
				return nil
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), strings.Join(target, "\n"))
			return err
		},
	})
	return func(args []string) ([]string, error) {
		if len(args) == 0 {
			return args, nil
		}
		index := aliasArgumentIndex(args[0])
		if len(args) <= index {
			return args, nil
		}
		target, err := lookup(args[index])
		if err != nil {
			return nil, err
		}
		if len(target) == 0 {
			return args, nil
		}
		expanded := make([]string, 0, len(args)+len(target)-1)
		expanded = append(expanded, args[:index]...)
		expanded = append(expanded, target...)
		return append(expanded, args[index+1:]...), nil
	}
}

func resolveAlias(root *cobra.Command, load func() (config.Settings, error), name string) ([]string, error) {
	if reservedAlias(root, name) {
		return nil, nil
	}
	settings, err := load()
	if err != nil {
		return nil, err
	}
	target := strings.Fields(settings.Aliases[name])
	if len(target) == 0 {
		return nil, nil
	}
	if !builtInCommand(root, target[0]) {
		return nil, fmt.Errorf("alias %q must target a Timber command, got %q", name, target[0])
	}
	return target, nil
}

func aliasArgumentIndex(first string) int {
	switch first {
	case cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return 1
	default:
		return 0
	}
}

func reservedAlias(root *cobra.Command, name string) bool {
	switch name {
	case "", "help":
		return true
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") {
		return true
	}
	return builtInCommand(root, name)
}

func builtInCommand(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if strings.HasPrefix(command.Name(), "_") {
			continue
		}
		if command.Name() == name || command.HasAlias(name) {
			return true
		}
	}
	return false
}
