package quotas

import (
	"context"
	"net/http"
	"time"
)

const (
	openCodeID           = "opencode-go"
	openCodeName         = "OpenCode Go"
	openCodeURL          = "https://opencode.ai/zen/go/v1/usage"
	openCodeRollingLabel = "5 hours"
)

func fetchOpenCode(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey(openCodeID, []string{"OPENCODE_GO_API_KEY"}, "OpenCode Go")
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, openCodeURL, bearer(key), readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something that is not a report."}
	}
	usage, _ := asObject(answer["usage"])
	var limits []Limit
	for _, window := range []struct {
		field, label string
		minutes      float64
		rolling      bool
	}{
		{"rolling", openCodeRollingLabel, 300, true},
		{"weekly", "Week", 10_080, false},
		{"monthly", "Month", 43_200, false},
	} {
		body, ok := asObject(usage[window.field])
		if !ok {
			continue
		}
		percent := percentOf(body["percent"])
		reset := time.Time{}
		if !window.rolling {
			reset = resetAt(body["resetsAt"])
		}
		limits = append(limits, Limit{
			Label:    window.label,
			Percent:  percent,
			ResetsAt: reset,
			Level:    LevelOf(percent, textOrEmpty(body["status"])),
			Window:   WindowDuration(window.minutes),
		})
	}
	return Report{Name: openCodeName, Limits: limits}, nil
}

// OmitOpenCodeRolling drops OpenCode Go's 5-hour window.
// That window rolls forward, and the report leaves it out until asked.
// Another service's 5-hour window is its own clock, so it stays.
func OmitOpenCodeRolling(snapshot Snapshot) Snapshot {
	reports := make([]Report, len(snapshot.Reports))
	for index, report := range snapshot.Reports {
		reports[index] = report
		if report.ID == openCodeID {
			reports[index].Limits = omitRollingLimits(report.Limits)
		}
	}
	snapshot.Reports = reports
	return snapshot
}

func omitRollingLimits(limits []Limit) []Limit {
	kept := make([]Limit, 0, len(limits))
	for _, limit := range limits {
		if openCodeRolling(limit) {
			continue
		}
		kept = append(kept, limit)
	}
	return kept
}

func openCodeRolling(limit Limit) bool {
	return limit.Label == openCodeRollingLabel
}
