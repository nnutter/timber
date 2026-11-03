// Package bump parses, validates, and bumps strict semantic version tags.
//
// A tag must match vMAJOR.MINOR.PATCH exactly: the v prefix is required,
// and pre-release and build metadata are rejected.
package bump

import (
	"cmp"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// strictPattern accepts only tags shaped like vMAJOR.MINOR.PATCH.
var strictPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Kind names the version component that a bump increments.
type Kind string

const (
	// Major increments the major version and resets minor and patch.
	Major Kind = "major"
	// Minor increments the minor version and resets patch.
	Minor Kind = "minor"
	// Patch increments the patch version.
	Patch Kind = "patch"
)

// Version is a parsed vMAJOR.MINOR.PATCH tag.
type Version struct {
	Major int
	Minor int
	Patch int
}

// Parse validates tag as a strict semantic version.
//
// Leading zeros and numbers too large for an int are rejected because
// they cannot round-trip through Version.
func Parse(tag string) (Version, error) {
	if !strictPattern.MatchString(tag) {
		return Version{}, fmt.Errorf("invalid semver tag %q: must match vMAJOR.MINOR.PATCH", tag)
	}
	number := func(part string) (int, error) {
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("invalid semver tag %q: %w", tag, err)
		}
		return value, nil
	}
	parts := strings.Split(tag[1:], ".")
	major, err := number(parts[0])
	if err != nil {
		return Version{}, err
	}
	minor, err := number(parts[1])
	if err != nil {
		return Version{}, err
	}
	patch, err := number(parts[2])
	if err != nil {
		return Version{}, err
	}
	version := Version{Major: major, Minor: minor, Patch: patch}
	if version.String() != tag {
		return Version{}, fmt.Errorf("invalid semver tag %q: must not use leading zeros", tag)
	}
	return version, nil
}

// String renders the version with its v prefix.
func (version Version) String() string {
	return "v" + strconv.Itoa(version.Major) + "." + strconv.Itoa(version.Minor) + "." + strconv.Itoa(version.Patch)
}

// Compare returns -1, 0, or 1 as left is older than, equal to, or newer
// than right.
func Compare(left Version, right Version) int {
	if order := cmp.Compare(left.Major, right.Major); order != 0 {
		return order
	}
	if order := cmp.Compare(left.Minor, right.Minor); order != 0 {
		return order
	}
	return cmp.Compare(left.Patch, right.Patch)
}

// Tag returns the tag that follows latest after applying kind.
func Tag(latest string, kind Kind) (string, error) {
	version, err := Parse(latest)
	if err != nil {
		return "", err
	}
	switch kind {
	case Major:
		return Version{Major: version.Major + 1}.String(), nil
	case Minor:
		return Version{Major: version.Major, Minor: version.Minor + 1}.String(), nil
	case Patch:
		return Version{Major: version.Major, Minor: version.Minor, Patch: version.Patch + 1}.String(), nil
	}
	return "", fmt.Errorf("unknown bump kind %q", kind)
}

// newerThan reports whether version ranks above highest.
func newerThan(version Version, highest Version) bool {
	return Compare(version, highest) > 0
}

// Highest returns the highest strict semantic version tag among tags, and
// whether it found one. Tags that are not strict semantic versions are
// skipped.
func Highest(tags []string) (string, bool) {
	highest := ""
	highestVersion := Version{}
	found := false
	for _, tag := range tags {
		version, err := Parse(tag)
		if err != nil {
			continue
		}
		if !found || newerThan(version, highestVersion) {
			highest, highestVersion, found = tag, version, true
		}
	}
	return highest, found
}
