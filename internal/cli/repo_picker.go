package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nnutter/timber/internal/timber"
)

type bubbleteaRepoPrompter struct{}

func (x bubbleteaRepoPrompter) Prompt(input io.Reader, repos []timber.RepoRef) (timber.RepoRef, error) {
	model := newRepoPickerModel(repos)
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if input != nil {
		programOptions = append(programOptions, tea.WithInput(input))
	}

	program := tea.NewProgram(model, programOptions...)
	finalModel, err := program.Run()
	if err != nil {
		return timber.RepoRef{}, fmt.Errorf("repository picker: %w", err)
	}

	result, ok := finalModel.(repoPickerModel)
	if !ok {
		return timber.RepoRef{}, errors.New("repository picker returned unexpected model")
	}
	if result.cancelled || result.choice.Name == "" {
		return timber.RepoRef{}, errors.New("repository selection cancelled")
	}
	return result.choice, nil
}

func newRepoPickerModel(repos []timber.RepoRef) repoPickerModel {
	items := make([]list.Item, 0, len(repos))
	for _, repo := range repos {
		items = append(items, repoListItem{repo: repo})
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false

	repoList := list.New(items, delegate, 0, 0)
	repoList.Title = "Select repository"
	repoList.SetShowStatusBar(false)
	repoList.SetFilteringEnabled(true)
	repoList.Styles.Title = lipgloss.NewStyle().Bold(true)

	return repoPickerModel{list: repoList}
}

type repoListItem struct {
	repo timber.RepoRef
}

func (x repoListItem) Description() string { return x.repo.BarePath }

func (x repoListItem) FilterValue() string { return x.repo.Name }

func (x repoListItem) Title() string { return x.repo.Name }

type repoPickerModel struct {
	list      list.Model
	choice    timber.RepoRef
	cancelled bool
}

func (x repoPickerModel) Init() tea.Cmd {
	return nil
}

func (x repoPickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		x.list.SetSize(message.Width, message.Height)
		return x, nil
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "esc", "q":
			if x.list.FilterState() != list.Filtering {
				x.cancelled = true
				return x, tea.Quit
			}
		case "enter":
			if x.list.FilterState() != list.Filtering {
				item, ok := x.list.SelectedItem().(repoListItem)
				if ok {
					x.choice = item.repo
				}
				return x, tea.Quit
			}
		}
	}

	var command tea.Cmd
	x.list, command = x.list.Update(message)
	return x, command
}

func (x repoPickerModel) View() string {
	return x.list.View()
}
