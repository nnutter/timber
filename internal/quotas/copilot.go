package quotas

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	copilotID   = "github-copilot"
	copilotName = "GitHub Copilot"
	copilotURL  = "https://api.github.com/copilot_internal/user"
)

func fetchCopilot(ctx context.Context, source Source, client *http.Client) (Report, error) {
	token, credentialSource, err := source.CopilotToken(ctx)
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, copilotURL, map[string]string{
		"Authorization":        "token " + token,
		"X-GitHub-Api-Version": "2025-04-01",
	}, readWait, credentialSource)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something this plugin could not read."}
	}

	limits, notes := copilotQuotas(answer)
	if len(limits) == 0 && noNotes(notes) {
		notes = append(notes, "GitHub did not report any limited quota data for this account.")
	}
	return Report{Name: copilotName, Limits: limits, Notes: notes}, nil
}

func copilotQuotas(answer map[string]any) ([]Limit, []string) {
	var limits []Limit
	var notes []string
	snapshots, _ := asObject(answer["quota_snapshots"])
	for _, field := range []struct{ key, label string }{
		{"premium_interactions", "Month"},
		{"chat", "Chat"},
		{"completions", "Completions"},
	} {
		snapshot, ok := asObject(snapshots[field.key])
		if !ok {
			continue
		}
		notes = copilotUnlimited(notes, field.key, snapshot)
		if limit, ok := copilotLimit(field.key, field.label, snapshot, answer); ok {
			limits = append(limits, limit)
		}
		notes = copilotOverage(notes, field.label, snapshot)
	}
	return limits, notes
}

func copilotUnlimited(notes []string, key string, snapshot map[string]any) []string {
	if !asBool(snapshot["unlimited"]) {
		return notes
	}
	if key != "premium_interactions" {
		return notes
	}
	return append(notes, "Premium requests are unlimited.")
}

func copilotLimit(key, label string, snapshot, answer map[string]any) (Limit, bool) {
	if asBool(snapshot["unlimited"]) {
		return Limit{}, false
	}
	if _, ok := asNumber(snapshot["percent_remaining"]); !ok {
		return Limit{}, false
	}
	remaining := min(100, percentOf(snapshot["percent_remaining"]))
	percent := 100 - remaining
	limit := Limit{
		Label:    label,
		Percent:  percent,
		ResetsAt: copilotReset(snapshot, answer),
		Level:    LevelOf(percent, ""),
	}
	return copilotCredits(limit, key, snapshot, answer), true
}

func copilotCredits(limit Limit, key string, snapshot, answer map[string]any) Limit {
	if key != "premium_interactions" {
		return limit
	}
	if !asBool(answer["token_based_billing"]) {
		return limit
	}
	credits, ok := asNumber(snapshot["remaining"])
	if !ok {
		return limit
	}
	limit.RemainingCredits = credits
	limit.HasRemainingCredits = true
	return limit
}

func copilotOverage(notes []string, label string, snapshot map[string]any) []string {
	if !asBool(snapshot["overage_permitted"]) {
		return notes
	}
	return append(notes, fmt.Sprintf("%s: paid overage is enabled.", label))
}

func copilotReset(snapshot, answer map[string]any) time.Time {
	if reset := resetAt(snapshot["quota_reset_at"]); !reset.IsZero() {
		return reset
	}
	return resetAt(answer["quota_reset_date"])
}

func noNotes(notes []string) bool { return len(notes) == 0 }
