package timber

import (
	"context"
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// WizardActionCreate creates the selected worktree.
const WizardActionCreate = "create"

// WizardActionOpen opens the selected worktree.
const WizardActionOpen = "open"

// WizardSelection is the wizard result.
type WizardSelection struct {
	Action       string
	RepoName     string
	WorktreeName string
	Cancelled    bool
}

// WizardPrompter shows the interactive worktree wizard.
type WizardPrompter interface {
	Prompt(io.Reader, io.Writer, []RepoRef, []WorktreeRef, bool) (WizardSelection, error)
}

type bubbleteaCreateWizardPrompter struct {
	interactive func() bool
}

// RunTUI runs the interactive worktree wizard. A cancelled wizard returns
// nil without creating or opening anything.
func (runtime Runtime) RunTUI(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool, prompter WizardPrompter) error {
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

	repoRefs := make([]RepoRef, 0, len(repos))
	for _, repo := range repos {
		repoRefs = append(repoRefs, repo.ref())
	}
	worktreeRefs := make([]WorktreeRef, 0, len(worktrees))
	for _, worktree := range worktrees {
		worktreeRefs = append(worktreeRefs, worktree.ref())
	}
	selection, err := prompter.Prompt(input, output, repoRefs, worktreeRefs, !noTitle)
	if err != nil || selection.Cancelled {
		return err
	}

	if selection.Action == WizardActionOpen {
		return runtime.openSelectedWorktree(ctx, output, selection)
	}
	return runtime.createSelectedWorktree(ctx, input, output, stdout, selection, herdr, noHerdr)
}

func (runtime Runtime) createSelectedWorktree(
	ctx context.Context,
	input io.Reader,
	output, stdout io.Writer,
	selection WizardSelection,
	herdr, noHerdr bool,
) error {
	worktreePath, _, err := runtime.CreateManagedWorktree(
		ctx,
		input,
		output,
		selection.RepoName,
		selection.WorktreeName,
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
	selection WizardSelection,
) error {
	repository, _, err := runtime.openRegisteredRepository(selection.RepoName)
	if err != nil {
		return err
	}
	worktrees, err := runtime.managedWorktreesFromRepository(repository, selection.RepoName)
	if err != nil {
		return err
	}
	worktree, err := managedWorktreeByName(worktrees, selection.WorktreeName)
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
	repos []RepoRef,
	worktrees []WorktreeRef,
	showTitle bool,
) (WizardSelection, error) {
	if !x.terminalIsInteractive(input) {
		return WizardSelection{}, errors.New("tui requires an interactive terminal")
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
			return WizardSelection{Cancelled: true}, nil
		}
		return WizardSelection{}, err
	}

	result, ok := finalModel.(*createWizardModel)
	if !ok {
		return WizardSelection{}, errors.New("create wizard returned unexpected model")
	}
	if result.cancelled {
		return WizardSelection{Cancelled: true}, nil
	}
	return result.selection, nil
}

func (x bubbleteaCreateWizardPrompter) terminalIsInteractive(input io.Reader) bool {
	if x.interactive != nil {
		return x.interactive()
	}
	return isInteractiveTerminal(input)
}
