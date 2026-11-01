package timber

import (
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

func (x githubIdentity) valid() bool {
	if !githubLoginPattern.MatchString(x.Username) || x.FetchedAt.IsZero() || x.FetchedAt.After(time.Now()) || !isLess(time.Since(x.FetchedAt), githubIdentityLifetime) {
		return false
	}
	for _, organization := range x.Organizations {
		if !githubLoginPattern.MatchString(organization) {
			return false
		}
	}
	return true
}
