package timber

func (x registeredRepo) alias(runtime Runtime, origin string) string {
	result, err := gitOutput(runtime, x.BarePath, "config", "--local", "--get", "timber.alias")
	if err == nil && result.stdout != "" {
		return result.stdout
	}
	return defaultRepoAliasFromRemote(origin)
}

type registeredRepo struct {
	Name     string
	BarePath string
}

func (x registeredRepo) originURL(runtime Runtime) string {
	result, err := gitOutput(runtime, x.BarePath, "remote", "get-url", remoteName)
	if err != nil {
		return ""
	}
	return result.stdout
}
