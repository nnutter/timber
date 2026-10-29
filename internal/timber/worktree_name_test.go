package timber

import (
	"errors"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstUnusedWorktreeNameSkipsTakenNames(t *testing.T) {
	t.Parallel()
	names := []string{"orbital-starship", "reusable-falcon"}
	generate := func() string {
		name := names[0]
		names = names[1:]
		return name
	}
	exists := func(name string) (bool, error) {
		return name == "orbital-starship", nil
	}

	name, err := firstUnusedWorktreeName(generate, exists)
	require.NoError(t, err)
	assert.Equal(t, "reusable-falcon", name)
}

func TestFirstUnusedWorktreeNameReturnsErrorWhenAllNamesTaken(t *testing.T) {
	t.Parallel()
	exists := func(string) (bool, error) {
		return true, nil
	}

	name, err := firstUnusedWorktreeName(func() string { return "orbital-starship" }, exists)
	require.Error(t, err)
	assert.Empty(t, name)
	assert.Contains(t, err.Error(), "unused worktree name")
}

func TestFirstUnusedWorktreeNameReturnsExistsError(t *testing.T) {
	t.Parallel()
	inspectErr := errors.New("stat failed")
	exists := func(string) (bool, error) {
		return false, inspectErr
	}

	name, err := firstUnusedWorktreeName(func() string { return "orbital-starship" }, exists)
	require.ErrorIs(t, err, inspectErr)
	assert.Empty(t, name)
}

func TestWorktreeNameWordListsAreValid(t *testing.T) {
	t.Parallel()
	wordPattern := regexp.MustCompile(`^[a-z]+$`)
	cases := []struct {
		kind  string
		words []string
	}{
		{kind: "adjectives", words: worktreeNameAdjectives},
		{kind: "nouns", words: worktreeNameNouns},
	}

	seen := make(map[string]string)
	for _, testCase := range cases {
		require.NotEmpty(t, testCase.words, testCase.kind)
		assert.True(t, slices.IsSorted(testCase.words), testCase.kind)
		assert.Equal(t, slices.Compact(slices.Clone(testCase.words)), testCase.words, testCase.kind)
		for _, word := range testCase.words {
			assert.True(t, wordPattern.MatchString(word), word)
			otherKind, exists := seen[word]
			assert.False(t, exists, "word %q is in both %s and %s", word, otherKind, testCase.kind)
			seen[word] = testCase.kind
		}
	}
}
