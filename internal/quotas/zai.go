package quotas

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	zaiID   = "zai"
	zaiName = "Z.ai (GLM Coding Plan)"
	zaiURL  = "https://api.z.ai/api/monitor/usage/quota/limit"
	zaiPlan = "https://api.z.ai/api/biz/subscription/list"
)

func fetchZAI(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey(zaiID, []string{"GLM_API_KEY", "ZAI_API_KEY", "Z_AI_API_KEY"}, "Z.ai")
	if err != nil {
		return Report{}, err
	}
	headers := bearer(key)
	decoded, err := getJSON(ctx, client, zaiURL, headers, readWait)
	if err != nil {
		return Report{}, err
	}
	body, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something that is not a report."}
	}
	if body["success"] == false {
		return Report{}, zaiRefused(body)
	}
	data, ok := asObject(body["data"])
	if !ok {
		data = body
	}
	limits, notes := zaiLimits(asRecords(data["limits"]))
	plan := zaiPlanName(ctx, client, headers)
	lead := "From Hermes's Z.ai key."
	if plan != "" {
		lead = plan + ", from Hermes's Z.ai key."
	}
	return Report{Name: zaiName, Limits: limits, Notes: append([]string{lead}, notes...)}, nil
}

func zaiRefused(body map[string]any) error {
	message, _ := asText(body["msg"])
	if strings.Contains(strings.ToLower(message), "coding plan") {
		return SaidError{Text: "This key has no GLM Coding Plan: subscribe at z.ai/subscribe."}
	}
	return SaidError{Text: "It refused to read the quota."}
}

func zaiLimits(entries []map[string]any) ([]Limit, []string) {
	var limits []Limit
	var counts []Limit
	var notes []string
	for _, entry := range entries {
		limit, count, note := zaiEntry(entry)
		if limit != nil {
			limits = append(limits, *limit)
		}
		if count != nil {
			counts = append(counts, *count)
		}
		if note != "" {
			notes = append(notes, note)
		}
	}
	return append(limits, counts...), notes
}

func zaiEntry(entry map[string]any) (*Limit, *Limit, string) {
	window := zaiWindow(entry["unit"], entry["number"])
	if window == "" {
		return nil, nil, ""
	}
	minutes := zaiMinutes(entry["unit"], entry["number"])
	switch zaiType(entry) {
	case "TOKENS_LIMIT", "CREDIT_LIMIT":
		return zaiTokenLimit(entry, window, minutes), nil, ""
	case "TIME_LIMIT":
		count, note := zaiTimeLimit(entry, window, minutes)
		return nil, count, note
	}
	return nil, nil, ""
}

func zaiType(entry map[string]any) string {
	entryType, _ := asText(entry["type"])
	if entryType == "" {
		entryType, _ = asText(entry["name"])
	}
	return entryType
}

func zaiTokenLimit(entry map[string]any, window string, minutes float64) *Limit {
	percent := percentOf(entry["percentage"])
	return new(Limit{
		Label:    window,
		Percent:  percent,
		ResetsAt: resetAtMillis(entry["nextResetTime"]),
		Level:    LevelOf(percent, ""),
		Window:   WindowDuration(minutes),
	})
}

func zaiTimeLimit(entry map[string]any, window string, minutes float64) (*Limit, string) {
	used := percentOf(entry["currentValue"])
	total := percentOf(entry["usage"])
	percent := percentOf(entry["percentage"])
	if entry["percentage"] == nil && aboveZero(total) {
		percent = used / total * 100
	}
	limit := new(Limit{
		Label:    fmt.Sprintf("Web tools, %g/%g", used, total),
		Percent:  percent,
		ResetsAt: resetAtMillis(entry["nextResetTime"]),
		Level:    LevelOf(percent, ""),
		Window:   WindowDuration(minutes),
	})
	note := fmt.Sprintf("Web tools: web search, web reader, and Zread calls, for each %s.", strings.ToLower(window))
	return limit, note
}

func zaiMinutes(unit, count any) float64 {
	unitNumber, countNumber, ok := wholePair(unit, count)
	if !ok {
		return 0
	}
	minutes := zaiUnitMinutes(int(unitNumber))
	if minutes == 0 {
		return 0
	}
	return minutes * countNumber
}

func zaiUnitMinutes(unit int) float64 {
	switch unit {
	case 3:
		return 60
	case 4:
		return 1440
	case 5:
		return 43_200
	case 6:
		return 10_080
	default:
		return 0
	}
}

func zaiWindow(unit, count any) string {
	unitNumber, unitOK := asNumber(unit)
	countNumber, countOK := asNumber(count)
	if unitOK && monthUnit(unitNumber) && ready(countOK) && wholeNumber(countNumber) && aboveZero(countNumber) {
		if countNumber == 1 {
			return "Month"
		}
		return fmt.Sprintf("%d months", int(countNumber))
	}
	minutes := zaiMinutes(unit, count)
	if minutes == 0 {
		return ""
	}
	return WindowLabel(minutes)
}

func zaiPlanName(ctx context.Context, client *http.Client, headers map[string]string) string {
	decoded, err := getJSON(ctx, client, zaiPlan, headers, 5*time.Second)
	if err != nil {
		return ""
	}
	plans, ok := asObject(decoded)
	if !ok {
		return ""
	}
	entries := asRecords(plans["data"])
	var chosen map[string]any
	for _, entry := range entries {
		if entry["status"] == "VALID" && asBool(entry["inCurrentPeriod"]) {
			chosen = entry
			break
		}
	}
	if chosen == nil {
		chosen = firstRecord(entries)
	}
	if chosen == nil {
		return ""
	}
	name, _ := asText(chosen["productName"])
	return name
}

func monthUnit(unit float64) bool { return int(unit) == 5 }

func wholePair(unit, count any) (float64, float64, bool) {
	unitNumber, unitOK := asNumber(unit)
	countNumber, countOK := asNumber(count)
	if !unitOK || missing(countOK) || !wholeNumber(unitNumber) || !wholeNumber(countNumber) || !aboveZero(countNumber) {
		return 0, 0, false
	}
	return unitNumber, countNumber, true
}
