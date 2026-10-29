package todo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoJSONNestedHierarchy(t *testing.T) {
	t.Parallel()

	contents := strings.Join([]string{
		"# TODO",
		"- [ ] Top task",
		"  - [ ] Child task",
		"    - [x] Grandchild task",
		"- [X] Done top",
		"- plain bullet",
	}, "\n") + "\n"

	output := todoJSONFromContents(contents)

	assert.Equal(t, todoJSONSummary{Incomplete: 2, Complete: 2}, output.Summary)
	require.Len(t, output.Todos, 4)

	assert.Equal(t, 2, output.Todos[0].ID)
	assert.Equal(t, "Top task", output.Todos[0].Title)
	assert.Nil(t, output.Todos[0].DependsOn)
	assert.False(t, output.Todos[0].Complete)

	require.NotNil(t, output.Todos[1].DependsOn)
	assert.Equal(t, 2, *output.Todos[1].DependsOn)
	assert.Equal(t, 3, output.Todos[1].ID)
	assert.Equal(t, "Child task", output.Todos[1].Title)

	require.NotNil(t, output.Todos[2].DependsOn)
	assert.Equal(t, 3, *output.Todos[2].DependsOn)
	assert.Equal(t, 4, output.Todos[2].ID)
	assert.True(t, output.Todos[2].Complete)

	assert.Equal(t, 5, output.Todos[3].ID)
	assert.Nil(t, output.Todos[3].DependsOn)
	assert.True(t, output.Todos[3].Complete)
}

func TestTodoJSONSiblingReturnsToParent(t *testing.T) {
	t.Parallel()

	contents := "- [ ] Parent\n  - [ ] First\n  - [ ] Second\n- [ ] Other\n"
	output := todoJSONFromContents(contents)

	require.Len(t, output.Todos, 4)
	assert.Nil(t, output.Todos[0].DependsOn)
	require.NotNil(t, output.Todos[1].DependsOn)
	assert.Equal(t, 1, *output.Todos[1].DependsOn)
	require.NotNil(t, output.Todos[2].DependsOn)
	assert.Equal(t, 1, *output.Todos[2].DependsOn)
	assert.Nil(t, output.Todos[3].DependsOn)
	assert.Equal(t, todoJSONSummary{Incomplete: 4}, output.Summary)
}

func TestTodoJSONEmptyContents(t *testing.T) {
	t.Parallel()

	output := todoJSONFromContents("# TODO\nno items\n")
	assert.Equal(t, todoJSONSummary{}, output.Summary)
	assert.NotNil(t, output.Todos)
	assert.Empty(t, output.Todos)
}
