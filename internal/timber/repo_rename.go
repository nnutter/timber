package timber

import (
	"os"
)

const repoRenamePathFileEnvVarName = "TIMBER_RENAME_PATH_FILE"

type (
	renamePathFunc      func(string, string) error
	repairWorktreesFunc func(string, []string) error
)

// RenameRepo renames a registered repository and its managed worktrees. It
// returns the source and destination names for CLI reporting.
func (runtime Runtime) RenameRepo(oldName, newName string) (string, string, error) {
	plan, err := runtime.buildRepositoryRenamePlan(oldName, newName)
	if err != nil {
		return "", "", err
	}

	if err := runtime.reportRenamedCurrentPath(plan.currentTargetDir); err != nil {
		return "", "", err
	}
	repair := func(barePath string, worktreePaths []string) error {
		return repairWorktrees(runtime, barePath, worktreePaths)
	}
	if err := plan.apply(os.Rename, repair); err != nil {
		return "", "", err
	}

	return plan.sourceRepo.Name, plan.destinationRepo.Name, nil
}
