package quotas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	grokID       = "xai"
	grokName     = "Grok"
	grokBilling  = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	grokSettings = "https://cli-chat-proxy.grok.com/v1/settings"
)

func fetchGrok(ctx context.Context, source Source, client *http.Client) (Report, error) {
	token, credentialSource, err := source.XAIToken()
	if err != nil {
		return Report{}, err
	}
	headers := map[string]string{
		"Authorization":    "Bearer " + token,
		"x-xai-token-auth": "xai-grok-cli", // #nosec G101 -- client name the Grok CLI sends, not a secret
	}
	decoded, err := getJSON(ctx, client, grokBilling, headers, readWait, credentialSource)
	if err != nil {
		return Report{}, grokError(err)
	}
	body, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something that is not a report."}
	}

	config, _ := asObject(body["config"])
	period, _ := asObject(config["currentPeriod"])
	start := resetAt(period["start"])
	end := resetAt(period["end"])
	var limits []Limit
	var notes []string
	if !start.IsZero() && !end.IsZero() {
		minutes := end.Sub(start).Minutes()
		if minutes > 0 {
			percent := percentOf(config["creditUsagePercent"])
			limits = append(limits, Limit{
				Label:    WindowLabel(minutes),
				Percent:  percent,
				ResetsAt: end,
				Level:    LevelOf(percent, ""),
				Window:   WindowDuration(minutes),
			})
		}
	}
	capObject, _ := asObject(config["onDemandCap"])
	if cap := percentOf(capObject["val"]); cap > 0 {
		notes = append(notes, fmt.Sprintf("Pay as you go: up to %g credits.", cap))
	}
	return Report{Name: grokPlanName(ctx, client, headers), Limits: limits, Notes: notes}, nil
}

func grokError(err error) error {
	if status, ok := errors.AsType[StatusError](err); ok && teamAccount(status) {
		return SaidError{Text: "Grok keeps no personal quota for a team account."}
	}
	return err
}

func grokPlanName(ctx context.Context, client *http.Client, headers map[string]string) string {
	decoded, err := getJSON(ctx, client, grokSettings, headers, 5*time.Second, "")
	if err != nil {
		return grokName
	}
	settings, ok := asObject(decoded)
	if !ok {
		return grokName
	}
	if name, ok := asText(settings["subscription_tier_display"]); ok {
		return name
	}
	return grokName
}

func teamAccount(status StatusError) bool {
	return status.Status == http.StatusPreconditionFailed
}
