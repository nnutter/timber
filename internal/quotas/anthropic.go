package quotas

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicID   = "anthropic"
	anthropicName = "Claude (Anthropic)"
	anthropicURL  = "https://api.anthropic.com/api/oauth/usage"
	sessionWindow = 5 * time.Hour
	weekWindow    = 7 * 24 * time.Hour
)

func fetchAnthropic(ctx context.Context, source Source, client *http.Client) (Report, error) {
	token, err := source.AnthropicPlanToken()
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, anthropicURL, map[string]string{
		"Authorization":  "Bearer " + token,
		"anthropic-beta": "oauth-2025-04-20",
	}, readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something that is not a report."}
	}

	var limits []Limit
	if entries, isList := answer["limits"].([]any); isList {
		for _, entry := range asRecords(entries) {
			percent := percentOf(entry["percent"])
			limits = append(limits, Limit{
				Label:    anthropicLabel(entry),
				Percent:  percent,
				ResetsAt: resetAt(entry["resets_at"]),
				Level:    LevelOf(percent, textOrEmpty(entry["severity"])),
				Binding:  asBool(entry["is_active"]),
				Window:   anthropicWindow(entry["kind"]),
			})
		}
	} else {
		for _, field := range []struct {
			key, label, kind string
		}{
			{"five_hour", "5-hour session", "session"},
			{"seven_day", "Week, all models", "weekly_all"},
		} {
			window, ok := asObject(answer[field.key])
			if !ok {
				continue
			}
			percent := percentOf(window["utilization"])
			limits = append(limits, Limit{
				Label:    field.label,
				Percent:  percent,
				ResetsAt: resetAt(window["resets_at"]),
				Level:    LevelOf(percent, ""),
				Window:   anthropicWindow(field.kind),
			})
		}
	}

	extra, _ := asObject(answer["extra_usage"])
	note := "Extra usage is off."
	if asBool(extra["is_enabled"]) {
		note = "Extra usage is on."
	}
	return Report{Name: anthropicName, Limits: limits, Notes: []string{note}}, nil
}

func anthropicLabel(entry map[string]any) string {
	kind, _ := entry["kind"].(string)
	switch kind {
	case "session":
		return "5-hour session"
	case "weekly_all":
		return "Week, all models"
	case "weekly_scoped":
		scope, _ := asObject(entry["scope"])
		model, _ := asObject(scope["model"])
		name, ok := asText(model["display_name"])
		if !ok {
			name = "one model"
		}
		return "Week, " + name
	default:
		if kind == "" {
			return "Limit"
		}
		return strings.ReplaceAll(kind, "_", " ")
	}
}

func anthropicWindow(kind any) time.Duration {
	if kind == "session" {
		return sessionWindow
	}
	return weekWindow
}

func textOrEmpty(value any) string {
	text, _ := asText(value)
	return text
}
