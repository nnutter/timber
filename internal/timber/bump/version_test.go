package bump

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAcceptsStrictSemver(t *testing.T) {
	version, err := Parse("v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, Version{Major: 1, Minor: 2, Patch: 3}, version)
	assert.Equal(t, "v1.2.3", version.String())
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "empty"},
		{name: "missing v prefix", tag: "1.2.3"},
		{name: "two components", tag: "v1.2"},
		{name: "four components", tag: "v1.2.3.4"},
		{name: "leading zero", tag: "v01.2.3"},
		{name: "prerelease", tag: "v1.2.3-rc.1"},
		{name: "build metadata", tag: "v1.2.3+meta"},
		{name: "not a version", tag: "release-foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.tag)
			require.Error(t, err)
		})
	}
}

func TestTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		kind Kind
		want string
	}{
		{name: "major resets minor and patch", tag: "v1.2.3", kind: Major, want: "v2.0.0"},
		{name: "minor resets patch", tag: "v1.2.3", kind: Minor, want: "v1.3.0"},
		{name: "patch", tag: "v1.2.3", kind: Patch, want: "v1.2.4"},
		{name: "patch carries", tag: "v1.2.9", kind: Patch, want: "v1.2.10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Tag(tt.tag, tt.kind)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTagRejects(t *testing.T) {
	t.Run("invalid tag", func(t *testing.T) {
		_, err := Tag("1.2.3", Patch)
		require.Error(t, err)
	})

	t.Run("unknown kind", func(t *testing.T) {
		_, err := Tag("v1.2.3", Kind("huge"))
		require.Error(t, err)
	})
}

func TestHighest(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{name: "single", tags: []string{"v1.2.3"}, want: "v1.2.3"},
		{name: "numeric not lexical", tags: []string{"v1.9.0", "v1.10.0"}, want: "v1.10.0"},
		{name: "highest across majors", tags: []string{"v1.9.0", "v2.0.0", "v1.10.0"}, want: "v2.0.0"},
		{name: "skips non-semver", tags: []string{"release-foo", "v1.2.3"}, want: "v1.2.3"},
		{name: "skips invalid semver", tags: []string{"v1.2.3-rc.1", "v1.2.2"}, want: "v1.2.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := Highest(tt.tags)
			require.True(t, found)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHighestWithoutUsableTag(t *testing.T) {
	for _, tags := range [][]string{nil, {}, {"release-foo"}, {"v1.2"}} {
		got, found := Highest(tags)
		assert.False(t, found)
		assert.Empty(t, got)
	}
}

func TestCompare(t *testing.T) {
	older := Version{Major: 1, Minor: 9, Patch: 0}
	newer := Version{Major: 1, Minor: 10, Patch: 0}
	assert.Equal(t, -1, Compare(older, newer))
	assert.Equal(t, 1, Compare(newer, older))
	assert.Equal(t, 0, Compare(newer, newer))
}
