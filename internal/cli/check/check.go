// Package check provides the Git history validation command.
package check

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/nnutter/timber/internal/process"
)

var unfinishedSubject = regexp.MustCompile(`(?i)^(?:(?:fixup|amend|reword|squash)!|(?:wip|todo|temp|tmp|draft)(?:[:\s]|$))`)

// New constructs the command that validates a range of commits.
func New(dir string) *cobra.Command {
	return &cobra.Command{
		Use:   "check [revision-range]",
		Short: "Check commit messages in a Git history range",
		Long:  "Check commit messages in a Git history range. The default range is @{upstream}..HEAD.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			revision := "@{upstream}..HEAD"
			if len(args) != 0 {
				revision = args[0]
			}
			const format = "--format=%h%x00%s%x00%b%x00%(trailers:key=Change-Id,valueonly)%x00%(trailers)"
			git := process.Command(cmd.Context(), "git", "log", "--reverse", "-z", format, "--end-of-options", revision, "--")
			git.Dir = dir
			output, err := git.Output()
			if err != nil {
				if exit, ok := errors.AsType[*exec.ExitError](err); ok {
					return fmt.Errorf("read Git range %q: %s", revision, strings.TrimSpace(string(exit.Stderr)))
				}
				return fmt.Errorf("read Git range %q: %w", revision, err)
			}
			return checkHistory(string(output))
		},
	}
}

func checkHistory(output string) error {
	if output == "" {
		return nil
	}
	fields := strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
	const recordWidth = 5
	if len(fields)%recordWidth != 0 {
		return errors.New("unexpected Git history output")
	}
	var failures []error
	for i := 0; i < len(fields); i += recordWidth {
		if problems := checkMessage(fields[i+1], fields[i+2], fields[i+3], fields[i+4]); len(problems) != 0 {
			failures = append(failures, fmt.Errorf("%s %q: %s", fields[i], fields[i+1], strings.Join(problems, "; ")))
		}
	}
	return errors.Join(failures...)
}

func checkMessage(subject, body, changeID, trailers string) []string {
	var problems []string
	if unfinishedSubject.MatchString(subject) {
		problems = append(problems, "unfinished commit subject")
	} else {
		problems = checkMessageFormat(subject, body, trailers)
	}
	if strings.TrimSpace(changeID) == "" {
		problems = append(problems, "missing Change-Id trailer")
	}
	return problems
}

func checkMessageFormat(subject, body, trailers string) []string {
	var problems []string
	first, _ := utf8.DecodeRuneInString(subject)
	if !unicode.IsUpper(first) {
		problems = append(problems, "subject must start with a capital letter")
	}
	if strings.HasSuffix(subject, ".") {
		problems = append(problems, "subject must not end with a period")
	}
	if utf8.RuneCountInString(subject) > 100 {
		problems = append(problems, "subject exceeds 100 characters")
	}
	body = strings.TrimSuffix(strings.TrimSpace(body), strings.TrimSpace(trailers))
	for line := range strings.SplitSeq(body, "\n") {
		if utf8.RuneCountInString(line) > 72 {
			problems = append(problems, "body line exceeds 72 characters")
			break
		}
	}
	return problems
}
