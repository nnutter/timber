package timber

const repoRenamePathFileEnvVarName = "TIMBER_RENAME_PATH_FILE"

type (
	renamePathFunc      func(string, string) error
	repairWorktreesFunc func(string, []string) error
)
