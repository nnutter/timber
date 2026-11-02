package timber

import (
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
