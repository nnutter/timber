package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

// Check every runnable example against Cobra's real command/flag definitions.
// No remote access, editor, or destructive operation is executed by this check.
func TestTimberSkillExamplesMatchCLI(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("..", "..", "skills", "timber", "SKILL.md"))
	require.NoError(t, err)
	text := string(content)
	require.True(t, strings.HasPrefix(text, "---\nname: timber\ndescription: "))
	sections := strings.SplitN(text, "\n---\n", 2)
	require.Len(t, sections, 2)
	for _, heading := range []string{"## When to use", "## Prerequisites and configuration", "## How to run", "## Procedure", "## Pitfalls and limitations", "## Verification"} {
		assert.Contains(t, sections[1], heading)
	}
	examples := 0
	inBash := false
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "```") {
			inBash = line == "```bash"
			continue
		}
		if !inBash || !strings.HasPrefix(line, "timber ") {
			continue
		}
		examples++
		t.Run(line, func(t *testing.T) {
			args, err := splitShellWords(line)
			require.NoError(t, err)
			args = args[1:]
			root := newRootCommand(timber.Runtime{})
			command, remaining, err := root.Find(args)
			require.NoError(t, err)
			require.NotSame(t, root, command, "example must resolve to a subcommand")
			require.NoError(t, command.ParseFlags(remaining))
			require.NoError(t, command.ValidateArgs(command.Flags().Args()))
		})
	}
	require.Positive(t, examples, "must validate at least one runnable example")
}

// splitShellWords splits an example line into arguments, keeping a value that
// the example quotes together so a title or body with spaces survives.
func splitShellWords(line string) ([]string, error) {
	var words []string
	var current strings.Builder
	var quote rune
	inWord := false
	for _, character := range line {
		switch {
		case quote != 0:
			if character == quote {
				quote = 0
				continue
			}
			current.WriteRune(character)
		case character == '\'' || character == '"':
			quote = character
			inWord = true
		case character == ' ' || character == '\t':
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
		default:
			current.WriteRune(character)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %q in example", string(quote))
	}
	if inWord {
		words = append(words, current.String())
	}
	return words, nil
}

func TestTimberSkillWorktreeWorkflow(t *testing.T) {
	t.Parallel()
	repository := newTestRepository(t)
	selector := at(testRepoName, "feature/skill")
	result := repository.runTimber(t, "repo", "list")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, testRepoName)
	result = repository.runTimber(t, "create", selector, "--no-herdr")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, repository.worktreePath("feature/skill"))
	result = repository.runTimber(t, "list", at(testRepoName, ""), "--json")
	require.NoError(t, result.err, result.stderr)
	assert.Contains(t, result.stdout, `"name":"feature/skill"`)
	result = repository.runTimber(t, "todo", selector, "--path")
	require.NoError(t, result.err, result.stderr)
	assert.FileExists(t, strings.TrimSpace(result.stdout))
	result = repository.runTimber(t, "todo", selector, "--json")
	require.NoError(t, result.err, result.stderr)
	result = repository.runTimber(t, "prune", at(testRepoName, ""), "--dry-run")
	require.NoError(t, result.err, result.stderr)
	repository.assertPathPresent(t, repository.worktreePath("feature/skill"))
	repository.mergeWorktreeBranch(t, "feature/skill")
	result = repository.runTimber(t, "remove", selector)
	require.NoError(t, result.err, result.stderr)
	repository.assertPathMissing(t, repository.worktreePath("feature/skill"))
	repository.assertBranchMissing(t, "feature/skill")
	result = repository.runTimber(t, "list", at(testRepoName, ""), "--json")
	require.NoError(t, result.err, result.stderr)
	assert.NotContains(t, result.stdout, `"name":"feature/skill"`)
}
