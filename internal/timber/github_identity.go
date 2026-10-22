package timber

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const githubIdentityLifetime = 24 * time.Hour

var githubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

type githubIdentity struct {
	Username      string    `json:"username"`
	Organizations []string  `json:"organizations"`
	FetchedAt     time.Time `json:"fetched_at"`
}

func (x githubIdentity) valid() bool {
	if !githubLoginPattern.MatchString(x.Username) || x.FetchedAt.IsZero() || x.FetchedAt.After(time.Now()) || time.Since(x.FetchedAt) >= githubIdentityLifetime {
		return false
	}
	for _, organization := range x.Organizations {
		if !githubLoginPattern.MatchString(organization) {
			return false
		}
	}
	return true
}

func (x githubIdentity) owns(owner string) bool {
	if strings.EqualFold(x.Username, owner) {
		return true
	}
	for _, organization := range x.Organizations {
		if strings.EqualFold(organization, owner) {
			return true
		}
	}
	return false
}

func (x Runtime) githubIdentity(ctx context.Context) (githubIdentity, error) {
	path := filepath.Join(x.xdgDataHome(), "timber", "github-identity.json")
	contents, cacheErr := os.ReadFile(path)
	if cacheErr == nil {
		var identity githubIdentity
		cacheErr = json.Unmarshal(contents, &identity)
		if cacheErr == nil {
			if identity.valid() {
				return identity, nil
			}
			cacheErr = errors.New("expired or invalid identity")
		}
	}

	// Bound the optional network lookup so worktree creation cannot wait forever.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	identity, err := x.fetchGitHubIdentity(ctx)
	if err != nil {
		return githubIdentity{}, fmt.Errorf("GitHub identity cache is unavailable (%v); refresh failed: %w", cacheErr, err)
	}
	contents, err = json.Marshal(identity)
	if err != nil {
		return githubIdentity{}, err
	}
	if err := writePrivateFile(path, contents); err != nil {
		return githubIdentity{}, fmt.Errorf("save GitHub identity cache: %w", err)
	}
	return identity, nil
}

func (x Runtime) fetchGitHubIdentity(ctx context.Context) (githubIdentity, error) {
	if _, err := x.runGh(ctx, x.CurrentDirectory, "auth", "status", "--hostname", "github.com"); err != nil {
		return githubIdentity{}, err
	}
	userJSON, err := x.runGh(ctx, x.CurrentDirectory, "api", "--hostname", "github.com", "user")
	if err != nil {
		return githubIdentity{}, err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(userJSON, &user); err != nil {
		return githubIdentity{}, fmt.Errorf("decode GitHub user: %w", err)
	}
	orgsJSON, err := x.runGh(ctx, x.CurrentDirectory, "api", "--hostname", "github.com", "--paginate", "--slurp", "user/orgs")
	if err != nil {
		return githubIdentity{}, err
	}
	var pages [][]struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(orgsJSON, &pages); err != nil {
		return githubIdentity{}, fmt.Errorf("decode GitHub organizations: %w", err)
	}
	identity := githubIdentity{Username: user.Login, FetchedAt: time.Now().UTC()}
	for _, page := range pages {
		for _, organization := range page {
			identity.Organizations = append(identity.Organizations, organization.Login)
		}
	}
	if !identity.valid() {
		return githubIdentity{}, errors.New("GitHub returned an invalid identity")
	}
	return identity, nil
}
