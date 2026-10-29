package timber

import (
	"context"
	"errors"
	"io"
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

// RunTUI runs the interactive worktree wizard using the supplied prompter.
// A cancelled wizard returns nil without creating or opening anything.
func (runtime Runtime) RunTUI(ctx context.Context, input io.Reader, output, stdout io.Writer, herdr, noHerdr, noTitle bool, prompter WizardPrompter, reporters CreateReporters) error {
	if prompter == nil {
		return errors.New("tui requires a wizard prompter")
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
		return runtime.openSelectedWorktree(ctx, output, selection, reporters)
	}
	return runtime.createSelectedWorktree(ctx, input, output, stdout, selection, herdr, noHerdr, reporters)
}

func (runtime Runtime) createSelectedWorktree(
	ctx context.Context,
	input io.Reader,
	output, stdout io.Writer,
	selection WizardSelection,
	herdr, noHerdr bool,
	reporters CreateReporters,
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
		reporters,
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
	reporters CreateReporters,
) error {
	if reporters.HerdrOpened == nil {
		return errors.New("tui requires a Herdr opening reporter")
	}
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
	return reporters.HerdrOpened(output, worktree.Name)
}
