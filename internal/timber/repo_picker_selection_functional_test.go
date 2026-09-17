package timber

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionalRepositoryPickerSelectsHighlightedRepository(t *testing.T) {
	t.Parallel()
	repos := []registeredRepo{{Name: "alpha", BarePath: "/repos/alpha.git"}, {Name: "beta", BarePath: "/repos/beta.git"}}
	model := newRepoPickerModel(repos)
	for _, message := range []tea.Msg{
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyDown},
	} {
		updated, _ := model.Update(message)
		model = updated.(repoPickerModel)
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(repoPickerModel)

	assert.Equal(t, repos[1], result.choice)
	assert.False(t, result.cancelled)
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
}
