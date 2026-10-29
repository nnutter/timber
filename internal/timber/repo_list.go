package timber

import repomodel "github.com/nnutter/timber/internal/timber/repo"

// ListRepos returns registered repositories in name order. When details is
// false, it avoids reading each repository's Git configuration.
func (x Runtime) ListRepos(details bool) ([]repomodel.Repo, error) {
	repos, err := x.listRegisteredRepos()
	if err != nil {
		return nil, err
	}
	result := make([]repomodel.Repo, 0, len(repos))
	for _, repo := range repos {
		item := repomodel.Repo{Name: repo.Name}
		if details {
			item.Origin = repo.originURL(x)
			item.Alias = repo.alias(x, item.Origin)
		}
		result = append(result, item)
	}
	return result, nil
}
