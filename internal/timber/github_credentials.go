package timber

import (
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	// githubHost is the only host Timber creates pull requests on.
	githubHost = "github.com"

	// ghKeyringServiceName is the keyring service the GitHub CLI stores
	// github.com tokens under, and ghHostsFileName is its config file, which
	// names the active user.
	ghKeyringServiceName = "gh:" + githubHost
	ghHostsFileName      = "hosts.yml"

	// keyringTimeout bounds a keyring lookup. A headless Linux session has no
	// Secret Service to answer, and a locked keyring can wait on a prompt; a
	// token lookup must not stall the command.
	keyringTimeout = 10 * time.Second
)

type keyringTimeoutError struct {
	timeout time.Duration
}

func (e *keyringTimeoutError) Error() string {
	return fmt.Sprintf("keyring lookup timed out after %s", e.timeout)
}

// lookupGitHubKeyring reads a secret from the system keyring. The lookup runs
// in its own goroutine so a keyring that never answers cannot hang Timber.
func lookupGitHubKeyring(service, user string) (string, error) {
	type result struct {
		secret string
		err    error
	}
	results := make(chan result, 1)
	go func() {
		secret, err := keyring.Get(service, user)
		results <- result{secret: secret, err: err}
	}()
	select {
	case result := <-results:
		return result.secret, result.err
	case <-time.After(keyringTimeout):
		return "", &keyringTimeoutError{timeout: keyringTimeout}
	}
}
