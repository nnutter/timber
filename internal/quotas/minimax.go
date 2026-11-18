package quotas

import (
	"context"
	"fmt"
	"net/http"
)

const (
	miniMaxID   = "minimax"
	miniMaxName = "MiniMax (Coding Plan)"
	miniMaxURL  = "https://api.minimax.io/v1/api/openplatform/coding_plan/remains"
)

func fetchMiniMax(ctx context.Context, source Source, client *http.Client) (Report, error) {
	token, _, err := source.MiniMaxToken()
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, miniMaxURL, bearer(token), readWait)
	if err != nil {
		return Report{}, err
	}
	body, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something that is not a report."}
	}
	if err := miniMaxRefusal(body); err != nil {
		return Report{}, err
	}
	limits, notes := miniMaxCollect(body)
	return Report{Name: miniMaxName, Limits: limits, Notes: notes}, nil
}

func miniMaxRefusal(body map[string]any) error {
	base, _ := asObject(body["base_resp"])
	code, codeOK := asNumber(base["status_code"])
	if codeOK && !refusedCode(code) {
		return nil
	}
	if code == 1004 {
		return SaidError{Text: "It refused the key: sign in again in Hermes."}
	}
	shown := "unknown"
	if codeOK {
		shown = fmt.Sprintf("%g", code)
	}
	return SaidError{Text: fmt.Sprintf("It refused to read the plan (code %s).", shown)}
}

func miniMaxCollect(body map[string]any) ([]Limit, []string) {
	var limits []Limit
	var others []Limit
	notes := []string{"From Hermes's MiniMax key."}
	for _, lane := range asRecords(body["model_remains"]) {
		laneLimits, laneOthers, laneNotes := miniMaxLane(lane)
		limits = append(limits, laneLimits...)
		others = append(others, laneOthers...)
		notes = append(notes, laneNotes...)
	}
	return append(limits, others...), notes
}

func miniMaxLane(lane map[string]any) ([]Limit, []Limit, []string) {
	textLane := lane["model_name"] == "general"
	var limits []Limit
	var others []Limit
	var notes []string
	for _, window := range miniMaxWindows(lane) {
		limit, note, skip := miniMaxWindow(lane, window, textLane)
		if note != "" {
			notes = append(notes, note)
		}
		if skip {
			continue
		}
		if limit == nil {
			continue
		}
		if textLane {
			limits = append(limits, *limit)
		} else {
			others = append(others, *limit)
		}
	}
	return limits, others, notes
}

func miniMaxWindow(lane, window map[string]any, textLane bool) (*Limit, string, bool) {
	minutes := miniMaxMinutes(window)
	if minutes <= 0 {
		return nil, "", true
	}
	label := WindowLabel(minutes)
	if miniMaxUnlimited(window) {
		return nil, miniMaxWeekNote(textLane, label), true
	}
	return miniMaxLimit(lane, window, textLane, label, minutes), "", false
}

func miniMaxUnlimited(window map[string]any) bool {
	total, _ := asNumber(window["total"])
	left, _ := asNumber(window["left"])
	status, _ := asNumber(window["status"])
	if status != 3 {
		return false
	}
	if !emptyCount(total) {
		return false
	}
	return unusedWeek(left)
}

func miniMaxWeekNote(textLane bool, label string) string {
	if textLane && weekLabel(label) {
		return "Text has no weekly limit."
	}
	return ""
}

func miniMaxWindows(lane map[string]any) []map[string]any {
	return []map[string]any{
		{
			"start": lane["start_time"], "end": lane["end_time"],
			"total": lane["current_interval_total_count"], "remaining": lane["current_interval_usage_count"],
			"left": lane["current_interval_remaining_percent"], "status": lane["current_interval_status"],
		},
		{
			"start": lane["weekly_start_time"], "end": lane["weekly_end_time"],
			"total": lane["current_weekly_total_count"], "remaining": lane["current_weekly_usage_count"],
			"left": lane["current_weekly_remaining_percent"], "status": lane["current_weekly_status"],
		},
	}
}

func miniMaxMinutes(window map[string]any) float64 {
	start, startOK := asNumber(window["start"])
	end, endOK := asNumber(window["end"])
	if !startOK || missing(endOK) {
		return 0
	}
	return (end - start) / 60_000
}

func miniMaxUsed(window map[string]any) (float64, bool) {
	if left, ok := asNumber(window["left"]); ok {
		return 100 - left, true
	}
	total, totalOK := asNumber(window["total"])
	remaining, remainingOK := asNumber(window["remaining"])
	if totalOK && aboveZero(total) && ready(remainingOK) {
		return (total - remaining) / total * 100, true
	}
	return 0, false
}

func miniMaxLimit(lane, window map[string]any, textLane bool, label string, minutes float64) *Limit {
	used, ok := miniMaxUsed(window)
	if !ok {
		return nil
	}
	prefix := ""
	if !textLane {
		name, nameOK := asText(lane["model_name"])
		if !nameOK {
			name = "other"
		}
		prefix = capitalize(name) + ", "
	}
	percent := min(100, percentOf(used))
	return new(Limit{
		Label:    prefix + label,
		Percent:  percent,
		ResetsAt: resetAtMillis(window["end"]),
		Level:    LevelOf(percent, ""),
		Window:   WindowDuration(minutes),
	})
}

func refusedCode(code float64) bool { return code != 0 }

func emptyCount(total float64) bool { return total <= 0 }

func unusedWeek(left float64) bool { return left >= 100 }

func weekLabel(label string) bool { return label == "Week" }
