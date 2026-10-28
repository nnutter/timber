package timber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			args := strings.Fields(line)[1:]
			root := newRootCommand(Runtime{})
			command, remaining, err := root.Find(args)
			require.NoError(t, err)
			require.NotSame(t, root, command, "example must resolve to a subcommand")
			require.NoError(t, command.ParseFlags(remaining))
			require.NoError(t, command.ValidateArgs(command.Flags().Args()))
		})
	}
	require.Positive(t, examples, "must validate at least one runnable example")
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
