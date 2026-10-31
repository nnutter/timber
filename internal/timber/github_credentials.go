package timber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
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

// githubToken returns an authentication token for the GitHub API.
//
// Sources are tried in order: GH_TOKEN, GITHUB_TOKEN, then the credential the
// GitHub CLI stored in the system keyring. Timber reads no token from a file:
// no configuration file holds a secret, and no error message repeats a token
// value.
func (x Runtime) githubToken() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if token := x.environmentValue(name); token != "" {
			return token, nil
		}
	}

	user, err := x.ghActiveUser()
	if err != nil {
		return "", err
	}
	// The keyring can hold an entry for the active user and an older entry
	// without one, so both slots are read.
	slots := []string{""}
	if user != "" {
		slots = []string{user, ""}
	}
	for _, slot := range slots {
		token, found, err := x.keyringToken(slot)
		if err != nil {
			return "", err
		}
		if found {
			return token, nil
		}
	}

	return "", fmt.Errorf("no GitHub token for %s; set GH_TOKEN or run: gh auth login", githubHost)
}

// environmentValue returns the last value set for name in the runtime
// environment, matching how the process would resolve it.
func (x Runtime) environmentValue(name string) string {
	prefix := name + "="
	for _, entry := range slices.Backward(x.Environment) {
		if value, found := strings.CutPrefix(entry, prefix); found {
			return value
		}
	}
	return ""
}

// ghConfigDirectory returns the directory the GitHub CLI reads its
// configuration from.
func (x Runtime) ghConfigDirectory() string {
	if x.GhConfigDirectory != "" {
		return x.GhConfigDirectory
	}
	if directory := x.environmentValue("GH_CONFIG_DIR"); directory != "" {
		return directory
	}
	return filepath.Join(x.ConfigHome, "gh")
}

// ghActiveUser returns the GitHub CLI user for github.com. The keyring slot
// for a token is named after that user.
func (x Runtime) ghActiveUser() (string, error) {
	path := filepath.Join(x.ghConfigDirectory(), ghHostsFileName)
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	// The file can also hold a token. Timber never reads it: a secret does not
	// belong in a configuration file.
	var hosts map[string]struct {
		User string `yaml:"user"`
	}
	if err := yaml.Unmarshal(contents, &hosts); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return hosts[githubHost].User, nil
}

// keyringToken reads a gh keyring entry. An unavailable keyring, an empty
// slot, and a timeout all just mean no stored token.
func (x Runtime) keyringToken(user string) (string, bool, error) {
	lookup := x.GitHubKeyringLookup
	if lookup == nil {
		lookup = lookupGitHubKeyring
	}
	token, err := lookup(ghKeyringServiceName, user)
	if err != nil {
		var timeout *keyringTimeoutError
		switch {
		case errors.Is(err, keyring.ErrNotFound), errors.As(err, &timeout):
			return "", false, nil
		default:
			return "", false, fmt.Errorf("read GitHub keyring entry: %w", err)
		}
	}
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

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
