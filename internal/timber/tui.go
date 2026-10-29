package timber

import (
	"context"
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	wizardActionCreate = "create"
	wizardActionOpen   = "open"

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

type createWizardSelection struct {
	action       string
	repoName     string
	worktreeName string
	cancelled    bool
}

type createWizardPrompter interface {
	Prompt(io.Reader, io.Writer, []registeredRepo, []managedWorktree, bool) (createWizardSelection, error)
}

type bubbleteaCreateWizardPrompter struct {
	interactive func() bool
}

// RunTUI runs the interactive worktree wizard. A cancelled wizard returns
// nil without creating or opening anything.
func (runtime Runtime) RunTUI(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool, prompter createWizardPrompter) error {
	if prompter == nil {
		prompter = bubbleteaCreateWizardPrompter{}
	}
	repos, err := runtime.listRegisteredReposForWizard()
	if err != nil {
		return err
	}

	worktrees, err := runtime.listWorktreesForWizard(repos)
	if err != nil {
		return err
	}

	selection, err := prompter.Prompt(input, output, repos, worktrees, !noTitle)
	if err != nil || selection.cancelled {
		return err
	}

	if selection.action == wizardActionOpen {
		return runtime.openSelectedWorktree(ctx, output, selection)
	}
	return runtime.createSelectedWorktree(ctx, input, output, stdout, selection, herdr, noHerdr)
}

func (runtime Runtime) createSelectedWorktree(
	ctx context.Context,
	input io.Reader,
	output, stdout io.Writer,
	selection createWizardSelection,
	herdr, noHerdr bool,
) error {
	worktreePath, _, err := runtime.CreateManagedWorktree(
		ctx,
		input,
		output,
		selection.repoName,
		selection.worktreeName,
		"",
		false,
		"",
		herdr,
		noHerdr,
	)
	if err != nil {
		return err
	}
	return runtime.ReportCreatedWorktreePath(stdout, worktreePath)
}

func (runtime Runtime) openSelectedWorktree(
	ctx context.Context,
	output io.Writer,
	selection createWizardSelection,
) error {
	repository, _, err := runtime.openRegisteredRepository(selection.repoName)
	if err != nil {
		return err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, selection.repoName)
	if err != nil {
		return err
	}
	worktree, err := managedWorktreeByName(worktrees, selection.worktreeName)
	if err != nil {
		return err
	}
	if err := runtime.openHerdrSpace(ctx, worktree); err != nil {
		return err
	}
	return reportOpenedHerdrSpace(output, worktree.Name)
}

func (x bubbleteaCreateWizardPrompter) Prompt(
	input io.Reader,
	output io.Writer,
	repos []registeredRepo,
	worktrees []managedWorktree,
	showTitle bool,
) (createWizardSelection, error) {
	if !x.terminalIsInteractive(input) {
		return createWizardSelection{}, errors.New("tui requires an interactive terminal")
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
			return createWizardSelection{cancelled: true}, nil
		}
		return createWizardSelection{}, err
	}

	result, ok := finalModel.(*createWizardModel)
	if !ok {
		return createWizardSelection{}, errors.New("create wizard returned unexpected model")
	}
	if result.cancelled {
		return createWizardSelection{cancelled: true}, nil
	}
	return result.selection, nil
}

func (x bubbleteaCreateWizardPrompter) terminalIsInteractive(input io.Reader) bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal(input)
}
