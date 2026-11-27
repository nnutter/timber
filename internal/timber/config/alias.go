package config

import (
	"fmt"
	"regexp"
	"strings"
)

var aliasName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func setAlias(settings *Settings, name string, values []string) error {
	if !aliasName.MatchString(name) {
		return fmt.Errorf("invalid alias %q: use lowercase letters, digits, and hyphens, starting with a letter", name)
	}
	if len(values) != 1 {
		return fmt.Errorf("alias %q takes one command string", name)
	}
	target := strings.Join(strings.Fields(values[0]), " ")
	if target == "" {
		return fmt.Errorf("alias %q needs a command", name)
	}
	if settings.Aliases == nil {
		settings.Aliases = make(map[string]string)
	}
	settings.Aliases[name] = target
	return nil
}
