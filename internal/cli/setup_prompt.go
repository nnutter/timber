package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/nnutter/timber/internal/timber"
)

func reportSetupWarning(output io.Writer, kind timber.SetupKind, cause error) error {
	var operation string
	switch kind {
	case timber.SetupMiseTrust:
		operation = "mise trust"
	case timber.SetupHKInstall:
		operation = "hk install"
	default:
		return fmt.Errorf("unknown automatic setup kind %q", kind)
	}
	_, err := fmt.Fprintf(output, "warning: automatic %s skipped: %v\n", operation, cause)
	return err
}

type setupPrompter struct{}

func (setupPrompter) Choose(input io.Reader, output io.Writer, kind timber.SetupKind, identity timber.SetupIdentity) (bool, error) {
	text, err := setupText(kind)
	if err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(output, text.warning); err != nil {
		return false, err
	}
	organizations := strings.Join(identity.Organizations, ", ")
	if organizations == "" {
		organizations = "none"
	}
	if _, err := fmt.Fprintf(output, "GitHub username: %s\nGitHub organizations: %s\n", identity.Username, organizations); err != nil {
		return false, err
	}
	reader := bufio.NewReader(input)
	for {
		if _, err := fmt.Fprint(output, text.question); err != nil {
			return false, err
		}
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("%s: %w", text.read, err)
		}
		decided, accept, err := setupAnswer(output, answer)
		if err != nil || isTrue(decided) {
			return accept, err
		}
	}
}

func (setupPrompter) ReportDisabled(output io.Writer, kind timber.SetupKind) error {
	text, err := setupText(kind)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, text.disabled)
	return err
}

func setupAnswer(output io.Writer, answer string) (bool, bool, error) {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, true, nil
	case "", "n", "no":
		return true, false, nil
	default:
		if _, err := fmt.Fprintln(output, "Enter yes or no."); err != nil {
			return true, false, err
		}
		return false, false, nil
	}
}

type setupPromptText struct {
	warning  string
	question string
	read     string
	disabled string
}

func setupText(kind timber.SetupKind) (setupPromptText, error) {
	switch kind {
	case timber.SetupMiseTrust:
		return setupPromptText{
			warning:  "This worktree qualifies for automatic mise trust. Trusting mise configuration permits code execution.",
			question: "Enable automatic mise trust for repositories owned by you or your GitHub organizations? [y/N] ",
			read:     "read mise trust choice",
			disabled: "Automatic mise trust remains disabled. Enable it later with: timber config --auto-mise-trust",
		}, nil
	case timber.SetupHKInstall:
		return setupPromptText{
			warning:  "This worktree qualifies for automatic hk hook installation. Installing hk hooks permits code execution.",
			question: "Enable automatic hk hook installation for repositories owned by you or your GitHub organizations? [y/N] ",
			read:     "read hk install choice",
			disabled: "Automatic hk hook installation remains disabled. Enable it later with: timber config --auto-hk-install",
		}, nil
	default:
		return setupPromptText{}, fmt.Errorf("unknown automatic setup kind %q", kind)
	}
}
