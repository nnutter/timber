package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/timber/internal/timber"
)

func TestFunctionalRepositoryPickerCancellationDoesNotSelectRepository(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyRunes, Runes: []rune("q")},
	} {
		t.Run(key.String(), func(t *testing.T) {
			model := newRepoPickerModel([]timber.RepoRef{{Name: "alpha"}})

			updated, command := model.Update(key)
			result := updated.(repoPickerModel)

			assert.True(t, result.cancelled)
			assert.Empty(t, result.choice.Name)
			require.NotNil(t, command)
			assert.Equal(t, tea.Quit(), command())
		})
	}
}
