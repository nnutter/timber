package quotas

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os/exec"
	"slices"

	json "encoding/json/v2"
)

const (
	codexID      = "openai-codex"
	codexName    = "ChatGPT"
	codexMissing = "The Codex CLI is not installed: install it, then run codex login."
)

func fetchCodex(ctx context.Context, source Source, _ *http.Client) (Report, error) {
	command, err := source.Command("QUOTAS_CODEX_COMMAND", "codex", codexMissing)
	if err != nil {
		return Report{}, err
	}
	answer, err := askCodex(ctx, command, source.userHome())
	if err != nil {
		return Report{}, err
	}
	snapshots := codexSnapshots(answer)
	limits, notes := codexLimits(snapshots, answer)
	return Report{Name: codexName, Limits: limits, Notes: notes}, nil
}

func askCodex(ctx context.Context, command, home string) (map[string]any, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, readWait, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))})
	defer cancel()

	process, stdin, stdout, err := startCodex(ctx, command, home)
	if err != nil {
		return nil, err
	}
	defer stopCodex(process)

	if err := writeLine(stdin, map[string]any{
		"method": "initialize",
		"id":     0,
		"params": map[string]any{
			"clientInfo": map[string]any{
				"name":    "timber_quotas",
				"title":   "Timber Quotas",
				"version": "1.0.0",
			},
			"capabilities": nil,
		},
	}); err != nil {
		return nil, SaidError{Text: "Codex could not be started."}
	}
	return readCodex(ctx, stdin, stdout)
}

func startCodex(ctx context.Context, command, home string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	process := exec.CommandContext(ctx, command, "app-server")
	process.Dir = home
	process.Stderr = io.Discard
	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, nil, nil, SaidError{Text: "Codex could not be started."}
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, nil, nil, SaidError{Text: "Codex could not be started."}
	}
	if err := process.Start(); err != nil {
		return nil, nil, nil, MissingError{Text: codexMissing}
	}
	return process, stdin, stdout, nil
}

func stopCodex(process *exec.Cmd) {
	_ = process.Process.Kill()
	_ = process.Wait()
}

func writeLine(out io.Writer, message map[string]any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", body)
	return err
}

func readCodex(ctx context.Context, stdin io.Writer, stdout io.Reader) (map[string]any, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), readLimit)
	for scanner.Scan() {
		if err := timedOut(ctx); err != nil {
			return nil, err
		}
		answer, ok := codexObject(scanner.Bytes())
		if !ok {
			continue
		}
		if _, notified := answer["method"]; notified {
			continue
		}
		switch messageID(answer["id"]) {
		case 0:
			if err := codexContinue(stdin); err != nil {
				return nil, err
			}
		case 1:
			return codexResult(answer)
		}
	}
	return nil, codexStopped(ctx)
}

func codexObject(body []byte) (map[string]any, bool) {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, false
	}
	return asObject(decoded)
}

func codexContinue(stdin io.Writer) error {
	if err := writeLine(stdin, map[string]any{"method": "initialized"}); err != nil {
		return SaidError{Text: "Codex stopped before it answered: run codex login, then try again."}
	}
	if err := writeLine(stdin, map[string]any{"method": "account/rateLimits/read", "id": 1}); err != nil {
		return SaidError{Text: "Codex stopped before it answered: run codex login, then try again."}
	}
	return nil
}

func codexResult(answer map[string]any) (map[string]any, error) {
	result, ok := asObject(answer["result"])
	if !ok {
		return nil, MissingError{Text: "Codex could not read the limits: run codex login, then try again."}
	}
	return result, nil
}

func codexStopped(ctx context.Context) error {
	if err := timedOut(ctx); err != nil {
		return err
	}
	return SaidError{Text: "Codex stopped before it answered: run codex login, then try again."}
}

func codexSnapshots(answer map[string]any) []map[string]any {
	if byID, ok := asObject(answer["rateLimitsByLimitId"]); ok && hasRates(byID) {
		keys := slices.Sorted(maps.Keys(byID))
		slices.SortStableFunc(keys, func(left, right string) int {
			return cmp.Compare(codexRank(left), codexRank(right))
		})
		values := make([]any, 0, len(keys))
		for _, key := range keys {
			values = append(values, byID[key])
		}
		if snapshots := asRecords(values); len(snapshots) > 0 {
			return snapshots
		}
	}
	if single, ok := asObject(answer["rateLimits"]); ok {
		return []map[string]any{single}
	}
	return nil
}

func codexLimits(snapshots []map[string]any, answer map[string]any) ([]Limit, []string) {
	return codexWindows(snapshots), codexNotes(snapshots, answer)
}

func codexWindows(snapshots []map[string]any) []Limit {
	var limits []Limit
	for _, snapshot := range snapshots {
		limits = append(limits, codexWindow(snapshot)...)
	}
	return limits
}

func codexWindow(snapshot map[string]any) []Limit {
	limitID, _ := snapshot["limitId"].(string)
	prefix := codexPrefix(snapshot, limitID)
	reached := snapshot["rateLimitReachedType"] != nil
	var limits []Limit
	for _, field := range []string{"primary", "secondary"} {
		window, ok := asObject(snapshot[field])
		if !ok {
			continue
		}
		limits = append(limits, codexWindowLimit(prefix, window, reached))
	}
	return limits
}

func codexPrefix(snapshot map[string]any, limitID string) string {
	if snapshot["limitId"] == nil || codexPlan(limitID) {
		return ""
	}
	name, ok := asText(snapshot["limitName"])
	if !ok {
		name = fmt.Sprint(snapshot["limitId"])
	}
	return name + ", "
}

func codexWindowLimit(prefix string, window map[string]any, reached bool) Limit {
	percent := percentOf(window["usedPercent"])
	minutes, _ := asNumber(window["windowDurationMins"])
	level := LevelOf(percent, "")
	if reached {
		level = LevelCritical
	}
	return Limit{
		Label:    prefix + WindowLabel(minutes),
		Percent:  percent,
		ResetsAt: resetAt(window["resetsAt"]),
		Level:    level,
		Window:   WindowDuration(minutes),
	}
}

func codexNotes(snapshots []map[string]any, answer map[string]any) []string {
	var notes []string
	if answer["ordinaryUsageAllowed"] == false {
		notes = append(notes, "Codex says the plan's usage is used up for now.")
	}
	notes = append(notes, codexCreditNote(snapshots)...)
	notes = append(notes, codexResetNote(answer)...)
	return notes
}

func codexCreditNote(snapshots []map[string]any) []string {
	for _, snapshot := range snapshots {
		credits, ok := asObject(snapshot["credits"])
		if !ok || !asBool(credits["hasCredits"]) {
			continue
		}
		return []string{codexCreditLine(credits)}
	}
	return nil
}

func codexCreditLine(credits map[string]any) string {
	if asBool(credits["unlimited"]) {
		return "Credits: unlimited."
	}
	balance, _ := asNumber(credits["balance"])
	return fmt.Sprintf("Credits: %g.", balance)
}

func codexResetNote(answer map[string]any) []string {
	resets, ok := asNumber(nested(answer, "rateLimitResetCredits", "availableCount"))
	if !ok || !someResets(resets) || !wholeResets(resets) {
		return nil
	}
	count := int(resets)
	word := "resets"
	if count == 1 {
		word = "reset"
	}
	return []string{fmt.Sprintf("%d free full %s available.", count, word)}
}

func codexRank(id string) int {
	if id == "codex" {
		return 0
	}
	return 1
}

func timedOut(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	if said, ok := errors.AsType[SaidError](context.Cause(ctx)); ok {
		return said
	}
	return SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))}
}

func messageID(value any) int {
	number, ok := asNumber(value)
	if !ok {
		return -1
	}
	return int(number)
}

func hasRates(byID map[string]any) bool { return len(byID) > 0 }

func codexPlan(limitID string) bool { return limitID == "codex" }

func someResets(resets float64) bool { return resets > 0 }

func wholeResets(resets float64) bool { return resets == float64(int(resets)) }

func nested(object map[string]any, key, field string) any {
	child, ok := asObject(object[key])
	if !ok {
		return nil
	}
	return child[field]
}
