package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Kind describes how a setting holds its value.
type Kind int

const (
	// KindBool holds true or false.
	KindBool Kind = iota
	// KindList holds a list of strings.
	KindList
)

// Key describes one setting a user may get and change. The *_decided keys that
// record a setup-prompt answer stay internal: set and remove update them.
type Key struct {
	Name        string
	Kind        Kind
	Description string

	get    func(*Settings) []string
	set    func(*Settings, []string) error
	append func(*Settings, []string) error
	remove func(*Settings, []string) error
}

// Keys returns the configurable settings in name order.
func Keys() []Key {
	return slices.Clone(keys)
}

// KeyNames returns the configurable setting names in name order.
func KeyNames() []string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.Name)
	}
	return names
}

// Get returns the values stored for name. A boolean setting always reports one
// value; a list setting reports none when it is empty.
func Get(settings Settings, name string) ([]string, error) {
	key, err := lookupKey(name)
	if err != nil {
		return nil, err
	}
	return key.get(&settings), nil
}

// Set replaces the value of name. A boolean setting takes one value; a list
// setting takes the values that replace it. Setting a prompted setting also
// records the answer, so its setup prompt stops asking.
func Set(settings *Settings, name string, values []string) error {
	key, err := lookupKey(name)
	if err != nil {
		return err
	}
	return key.set(settings, values)
}

// Append adds values to a list setting. A boolean setting has nothing to
// append to.
func Append(settings *Settings, name string, values []string) error {
	key, err := lookupKey(name)
	if err != nil {
		return err
	}
	return key.append(settings, values)
}

// Remove resets name to its default, or removes matching values from a list
// setting. Boolean settings take no values because they always hold one.
func Remove(settings *Settings, name string, values []string) error {
	key, err := lookupKey(name)
	if err != nil {
		return err
	}
	return key.remove(settings, values)
}

var keys = []Key{
	boolSetting(
		"auto_hk_install",
		"Install hk hooks in worktrees owned by you or your GitHub organizations",
		func(settings *Settings) *bool { return &settings.AutoHKInstall },
		func(settings *Settings) *bool { return &settings.AutoHKInstallDecided },
	),
	boolSetting(
		"auto_mise_trust",
		"Trust mise.toml in worktrees owned by you or your GitHub organizations",
		func(settings *Settings) *bool { return &settings.AutoMiseTrust },
		func(settings *Settings) *bool { return &settings.AutoMiseTrustDecided },
	),
	listSetting(
		"strip_github_owners",
		"GitHub owners whose repositories register under the short name",
		func(settings *Settings) *[]string { return &settings.StripGitHubOwners },
		UpdateStripGitHubOwners,
	),
}

// boolSetting describes a boolean setting. decided names the key that records
// the setup-prompt answer, so set and remove keep it in step; a nil decided
// means the setting has no prompt.
func boolSetting(name, description string, value, decided func(*Settings) *bool) Key {
	return Key{
		Name:        name,
		Kind:        KindBool,
		Description: description,
		get: func(settings *Settings) []string {
			return []string{strconv.FormatBool(*value(settings))}
		},
		set: func(settings *Settings, values []string) error {
			parsed, err := boolValue(name, values)
			if err != nil {
				return err
			}
			*value(settings) = parsed
			if decided != nil {
				*decided(settings) = true
			}
			return nil
		},
		append: func(*Settings, []string) error {
			return fmt.Errorf("setting %q is a boolean; only list settings can be appended", name)
		},
		remove: func(settings *Settings, values []string) error {
			if len(values) != 0 {
				return fmt.Errorf("setting %q is a boolean; use set %s false to clear it", name, name)
			}
			*value(settings) = false
			if decided != nil {
				*decided(settings) = false
			}
			return nil
		},
	}
}

// listSetting describes a list setting. normalize validates incoming values and
// applies them to the stored list.
func listSetting(name, description string, list func(*Settings) *[]string, normalize func([]string, []string, []string) ([]string, error)) Key {
	return Key{
		Name:        name,
		Kind:        KindList,
		Description: description,
		get: func(settings *Settings) []string {
			return slices.Clone(*list(settings))
		},
		set: func(settings *Settings, values []string) error {
			normalized, err := normalize(nil, values, nil)
			if err != nil {
				return err
			}
			*list(settings) = normalized
			return nil
		},
		append: func(settings *Settings, values []string) error {
			normalized, err := normalize(*list(settings), values, nil)
			if err != nil {
				return err
			}
			*list(settings) = normalized
			return nil
		},
		remove: func(settings *Settings, values []string) error {
			current := *list(settings)
			if len(values) == 0 {
				// Removing the setting clears the list.
				current = nil
			}
			normalized, err := normalize(current, nil, values)
			if err != nil {
				return err
			}
			*list(settings) = normalized
			return nil
		},
	}
}

// boolValue parses the single value a boolean setting takes.
func boolValue(name string, values []string) (bool, error) {
	if len(values) != 1 {
		return false, fmt.Errorf("setting %q takes one value, true or false", name)
	}
	parsed, err := strconv.ParseBool(values[0])
	if err != nil {
		return false, fmt.Errorf("setting %q needs true or false, got %q", name, values[0])
	}
	return parsed, nil
}

func lookupKey(name string) (Key, error) {
	for _, key := range keys {
		if key.Name == name {
			return key, nil
		}
	}
	return Key{}, fmt.Errorf("unknown config setting %q; valid settings are: %s", name, strings.Join(KeyNames(), ", "))
}
