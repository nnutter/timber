package timber

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type repoAddCommandOptions struct {
	runtime Runtime
	name    string
	alias   string
}

func NewRepoAddCommand(runtime Runtime) *cobra.Command {
	options := &repoAddCommandOptions{runtime: runtime}

	command := &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Register a bare repository from a remote URL or path",
		Args:  cobra.ExactArgs(1),
		RunE:  options.Execute,
	}
	command.Flags().StringVar(&options.name, "name", "", "Repository name (default: derived from URL)")
	command.Flags().StringVar(&options.alias, "alias", "", "Repository display alias (default: derived from origin)")

	return command
}

func (x *repoAddCommandOptions) Execute(command *cobra.Command, args []string) error {
	remoteURL, err := x.resolveAddRemoteURL(command.Context(), args[0])
	if err != nil {
		return err
	}

	repoName := normalizeRepoName(x.name)
	if repoName == "" {
		repoName, err = defaultRepoNameFromRemote(remoteURL)
		if err != nil {
			return err
		}
	}
	if err := validateRepoName(repoName); err != nil {
		return err
	}

	targetPath := x.runtime.bareRepoPath(repoName)
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("repository %q already exists at %s", repoName, targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect repository path %q: %w", targetPath, err)
	}

	if err := ensureDirectory(filepath.Dir(targetPath)); err != nil {
		return err
	}

	if _, err := gitOutput(x.runtime, x.runtime.CurrentDirectory, "clone", "--bare", remoteURL, targetPath); err != nil {
		return err
	}

	if err := configureBareOriginTracking(x.runtime, targetPath); err != nil {
		return err
	}

	if x.alias != "" {
		if _, err := gitOutput(x.runtime, targetPath, "config", "--local", "timber.alias", x.alias); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(command.ErrOrStderr(), "%s\n", statusStyle.Render("added repository "+repoName+" at "+targetPath))
	return err
}

// resolveAddRemoteURL maps user input to the clone URL for registration.
// GitHub short forms resolve to HTTPS when the repository is public and to
// SSH (without a .git suffix) when the GitHub API reports it as private.
// Anything inconclusive falls back to HTTPS.
func (x *repoAddCommandOptions) resolveAddRemoteURL(ctx context.Context, input string) (string, error) {
	remoteURL, err := resolveRemoteURL(input)
	if err != nil {
		return "", err
	}
	short, ok := parseGitHubShortForm(input)
	if !ok {
		return remoteURL, nil
	}
	public, known := x.runtime.githubRepoIsPublic(ctx, short)
	if !known || public {
		return "https://github.com/" + short, nil
	}
	return "git@github.com:" + short, nil
}
