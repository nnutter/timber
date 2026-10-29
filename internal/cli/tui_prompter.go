package cli

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nnutter/timber/internal/terminal"
	"github.com/nnutter/timber/internal/timber"
)

const (
	tuiWorktreeInputMaxLength = 72
	tuiWorktreeTitle          = "Open or Create Worktree"
	tuiWorktreeLegend         = "tab/↓ next • shift+tab/↑ prev • enter accept • esc/^c quit • worktree@repo creates"
)

var (
	tuiWorktreeTitleStyle    = lipgloss.NewStyle().Bold(true)
	tuiWorktreeLegendStyle   = lipgloss.NewStyle().Faint(true)
	tuiWorktreeInputBoxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	tuiWorktreeViewStyle     = lipgloss.NewStyle().Padding(1, 2)
)

type bubbleteaCreateWizardPrompter struct{}

func (x bubbleteaCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []timber.RepoRef,
	worktrees []timber.WorktreeRef,
	showTitle bool,
) (timber.WizardSelection, error) {
	if !terminal.IsInteractive(input) {
		return timber.WizardSelection{}, errors.New("tui requires an interactive terminal")
	}

	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if input != nil {
		programOptions = append(programOptions, tea.WithInput(input))
	}
	if output != nil {
		programOptions = append(programOptions, tea.WithOutput(output))
	}
	finalModel, err := tea.NewProgram(newCreateWizardModel(repos, worktrees, showTitle), programOptions...).Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return timber.WizardSelection{Cancelled: true}, nil
		}
		return timber.WizardSelection{}, err
	}

	result, ok := finalModel.(*createWizardModel)
	if !ok {
		return timber.WizardSelection{}, errors.New("create wizard returned unexpected model")
	}
	if result.cancelled {
		return timber.WizardSelection{Cancelled: true}, nil
	}
	return result.selection, nil
}
