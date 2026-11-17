package config

import (
	"fmt"
	"slices"
	"strings"
)

// UpdateStripGitHubOwners returns the owner list with add appended and remove
// dropped. GitHub logins are case-insensitive, so matching ignores case and the
// result keeps one spelling per owner. The result is sorted so the
// configuration file stays stable across edits.
func UpdateStripGitHubOwners(current, add, remove []string) ([]string, error) {
	additions, err := normalizeGitHubOwners(add)
	if err != nil {
		return nil, err
	}
	removals, err := normalizeGitHubOwners(remove)
	if err != nil {
		return nil, err
	}

	owners := make([]string, 0, len(current)+len(additions))
	for _, value := range current {
		owner := strings.TrimSpace(value)
		if isEmpty(owner) || containsGitHubOwner(removals, owner) || containsGitHubOwner(owners, owner) {
			continue
		}
		owners = append(owners, owner)
	}
	for _, owner := range additions {
		if !containsGitHubOwner(owners, owner) {
			owners = append(owners, owner)
		}
	}
	slices.SortFunc(owners, func(left, right string) int {
		return strings.Compare(strings.ToLower(left), strings.ToLower(right))
	})
	return owners, nil
}

func normalizeGitHubOwners(values []string) ([]string, error) {
	owners := make([]string, 0, len(values))
	for _, value := range values {
		owner := strings.TrimSpace(value)
		if isEmpty(owner) {
			return nil, fmt.Errorf("a GitHub owner must not be empty")
		}
		if strings.ContainsAny(owner, "/ \t") {
			return nil, fmt.Errorf("expected a single GitHub owner, got %q", owner)
		}
		owners = append(owners, owner)
	}
	return owners, nil
}

func containsGitHubOwner(owners []string, owner string) bool {
	for _, candidate := range owners {
		if strings.EqualFold(candidate, owner) {
			return true
		}
	}
	return false
}
