package quotas

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	json "encoding/json/v2"
)

const (
	googleID          = "google-ai"
	googleName        = "Google AI"
	googleMissing     = "The Antigravity CLI is not installed: install it, then sign in with agy."
	googleAuthMissing = "Sign in with the Antigravity CLI (`agy`) to read Google's plan."
	googleModelsLead  = "Models within this group: "
	dayWindow         = 24 * time.Hour
	monthWindow       = 30 * 24 * time.Hour
)

func fetchGoogle(ctx context.Context, source Source, _ *http.Client) ([]Report, error) {
	command, err := source.Command("QUOTAS_AGY_COMMAND", "agy", googleMissing)
	if err != nil {
		return nil, err
	}
	output, err := runAgyUsage(ctx, command, source.userHome())
	if err != nil {
		return nil, err
	}
	return parseGoogleReports(output)
}

func runAgyUsage(ctx context.Context, command, home string) ([]byte, error) {
	timeoutCtx, cancelTimeout := context.WithTimeoutCause(ctx, readWait, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))})
	defer cancelTimeout()
	runCtx, cancelRun := context.WithCancelCause(timeoutCtx)
	defer cancelRun(nil)

	process := exec.CommandContext(runCtx, command, "-p", "/usage", "--output-format", "json")
	process.Dir = home
	process.Stdin = nil
	process.WaitDelay = time.Second
	var stdout bytes.Buffer
	process.Stdout = &stdout
	stderr, err := process.StderrPipe()
	if err != nil {
		return nil, SaidError{Text: "Antigravity could not be started."}
	}
	if err := process.Start(); err != nil {
		return nil, MissingError{Text: googleMissing}
	}

	var watch sync.WaitGroup
	watch.Go(func() {
		watchAgyStderr(stderr, cancelRun)
	})
	watch.Wait()
	waitErr := process.Wait()
	return agyOutput(runCtx, stdout.Bytes(), waitErr)
}

func watchAgyStderr(stderr io.Reader, cancel context.CancelCauseFunc) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 16*1024), readLimit)
	for scanner.Scan() {
		if agyAuthText(scanner.Text()) {
			cancel(MissingError{Text: googleAuthMissing})
			return
		}
	}
}

func agyOutput(ctx context.Context, output []byte, waitErr error) ([]byte, error) {
	if err := agyContextError(ctx); err != nil {
		return nil, err
	}
	if waitErr != nil {
		return nil, agyExitError(output, waitErr)
	}
	return output, nil
}

func agyContextError(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	cause := context.Cause(ctx)
	if missingErr, ok := errors.AsType[MissingError](cause); ok {
		return missingErr
	}
	if said, ok := errors.AsType[SaidError](cause); ok {
		return said
	}
	return SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))}
}

func agyExitError(output []byte, waitErr error) error {
	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok && noAccountQuotaExit(exitErr) {
		return MissingError{Text: googleAuthMissing}
	}
	if agyAuthOutput(output) {
		return MissingError{Text: googleAuthMissing}
	}
	return SaidError{Text: "The Antigravity CLI could not read Google's plan."}
}

func parseGoogleReports(output []byte) ([]Report, error) {
	var decoded any
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, SaidError{Text: "The Antigravity CLI did not answer with a valid report."}
	}
	answer, ok := asObject(decoded)
	if !ok {
		return nil, SaidError{Text: "The Antigravity CLI did not answer with a valid report."}
	}
	if err := googleAnswerError(answer); err != nil {
		return nil, err
	}
	data, ok := googleUsageData(answer)
	if !ok {
		return nil, SaidError{Text: "The Antigravity CLI did not answer with its usage."}
	}
	groups := asRecords(data["groups"])
	usedIDs := make(map[string]int, len(groups))
	reports := make([]Report, 0, len(groups))
	for index, group := range groups {
		limits := googleLimits(asRecords(group["buckets"]))
		if len(limits) == 0 {
			continue
		}
		groupName, _ := asText(group["name"])
		reports = append(reports, Report{
			ID:     googleGroupID(groupName, index, usedIDs),
			Name:   googleGroupName(groupName),
			Limits: limits,
			Notes:  googleGroupNotes(group),
		})
	}
	return reports, nil
}

func googleAnswerError(answer map[string]any) error {
	status, _ := asText(answer["status"])
	if !strings.EqualFold(status, "error") {
		return nil
	}
	if agyAuthText(textOrEmpty(answer["error"])) {
		return MissingError{Text: googleAuthMissing}
	}
	return SaidError{Text: "The Antigravity CLI could not read Google's plan."}
}

func googleUsageData(answer map[string]any) (map[string]any, bool) {
	command, ok := asObject(answer["command"])
	if !ok {
		return nil, false
	}
	return asObject(command["data"])
}

func googleLimits(buckets []map[string]any) []Limit {
	limits := make([]Limit, 0, len(buckets))
	for _, bucket := range buckets {
		if limit, ok := googleBucketLimit(bucket); ok {
			limits = append(limits, limit)
		}
	}
	slices.SortStableFunc(limits, compareLimitWindow)
	return limits
}

func compareLimitWindow(left, right Limit) int {
	leftKnown := hasWindow(left)
	rightKnown := hasWindow(right)
	if leftKnown != rightKnown {
		if leftKnown {
			return -1
		}
		return 1
	}
	return cmp.Compare(left.Window, right.Window)
}

func googleBucketLimit(bucket map[string]any) (Limit, bool) {
	reset := resetAt(bucket["reset_time"])
	remaining, hasRemaining := asNumber(bucket["remaining_fraction"])
	if !hasRemaining {
		if reset.IsZero() {
			return Limit{}, false
		}
		remaining = 0
	}
	remaining = min(1, max(0, remaining))
	percent := (1 - remaining) * 100
	label, window := googleWindow(bucket)
	return Limit{
		Label:    label,
		Percent:  percent,
		ResetsAt: reset,
		Level:    LevelOf(percent, ""),
		Window:   window,
	}, true
}

func googleWindow(bucket map[string]any) (string, time.Duration) {
	windowText, _ := asText(bucket["window"])
	switch strings.ToLower(windowText) {
	case "5h":
		return "5 hours", sessionWindow
	case "weekly":
		return "Week", weekWindow
	case "daily", "24h":
		return "Day", dayWindow
	case "monthly":
		return "Month", monthWindow
	}
	if minutes, ok := googleWindowMinutes(windowText); ok {
		return WindowLabel(minutes), WindowDuration(minutes)
	}
	display, _ := asText(bucket["name"])
	return cmp.Or(display, windowText, "Limit"), 0
}

func googleWindowMinutes(text string) (float64, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if len(text) < 2 {
		return 0, false
	}
	unit := text[len(text)-1]
	amount, err := strconv.ParseFloat(text[:len(text)-1], 64)
	if err != nil || nonPositive(amount) {
		return 0, false
	}
	switch unit {
	case 'm':
		return amount, true
	case 'h':
		return amount * 60, true
	case 'd':
		return amount * 1440, true
	default:
		return 0, false
	}
}

func googleGroupID(groupName string, index int, used map[string]int) string {
	slug := googleSlug(groupName)
	base := fmt.Sprintf("%s-%d", googleID, index+1)
	if slug != "" {
		base = googleID + "-" + slug
	}
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, used[base])
}

func googleSlug(name string) string {
	var out strings.Builder
	dashed := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dashed && hasBuilderText(&out) {
				out.WriteByte('-')
			}
			out.WriteRune(r)
			dashed = false
			continue
		}
		dashed = true
	}
	return out.String()
}

func googleGroupName(groupName string) string {
	if groupName == "" {
		return googleName
	}
	return fmt.Sprintf("%s (%s)", googleName, groupName)
}

func googleGroupNotes(group map[string]any) []string {
	description, ok := asText(group["description"])
	if !ok {
		return nil
	}
	models, found := strings.CutPrefix(description, googleModelsLead)
	models = strings.TrimSpace(models)
	if !found || blank(models) {
		return nil
	}
	return []string{"Models: " + models}
}

func agyAuthOutput(output []byte) bool {
	var decoded any
	if err := json.Unmarshal(output, &decoded); err != nil {
		return agyAuthText(string(output))
	}
	answer, ok := asObject(decoded)
	if !ok {
		return false
	}
	return agyAuthText(textOrEmpty(answer["error"]))
}

func agyAuthText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "authentication required") || strings.Contains(lower, "authentication failed")
}

func noAccountQuotaExit(err *exec.ExitError) bool { return err.ExitCode() == 2 }

func hasBuilderText(builder *strings.Builder) bool { return builder.Len() > 0 }
