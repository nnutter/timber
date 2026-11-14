package quotas

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	json "encoding/json/v2"
)

const (
	openCodeID   = "opencode-go"
	openCodeName = "OpenCode Go"
	openCodeURL  = "https://opencode.ai/zen/go/v1/usage"

	deepSeekID   = "deepseek"
	deepSeekName = "DeepSeek"
	deepSeekURL  = "https://api.deepseek.com/user/balance"

	ollamaID   = "ollama"
	ollamaName = "Ollama Cloud"
	ollamaURL  = "https://ollama.com/api/balance"

	zaiID   = "zai"
	zaiName = "Z.ai (GLM Coding Plan)"
	zaiURL  = "https://api.z.ai/api/monitor/usage/quota/limit"
	zaiPlan = "https://api.z.ai/api/biz/subscription/list"

	miniMaxID   = "minimax"
	miniMaxName = "MiniMax (Coding Plan)"
	miniMaxURL  = "https://api.minimax.io/v1/api/openplatform/coding_plan/remains"

	museID      = "muse"
	museName    = "Muse Code"
	museMissing = "The Muse CLI is not installed: install it, then run muse login."
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
		{"rolling", "5 hours", 300, true},
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

func fetchDeepSeek(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey(deepSeekID, []string{"DEEPSEEK_API_KEY"}, "DeepSeek")
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, deepSeekURL, bearer(key), readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something this plugin could not read."}
	}
	infos := asRecords(answer["balance_infos"])
	var main map[string]any
	for _, info := range infos {
		if info["currency"] == "CNY" {
			main = info
			break
		}
	}
	if main == nil {
		main = firstRecord(infos)
	}
	if main == nil {
		return Report{}, SaidError{Text: "The answer named no balance."}
	}
	total := percentOf(main["total_balance"])
	currency, _ := asText(main["currency"])
	enough := deepSeekEnough(answer, total)
	return Report{
		Name: deepSeekName,
		Balance: new(Balance{
			Currency: currency,
			Total:    total,
			Granted:  percentOf(main["granted_balance"]),
			ToppedUp: percentOf(main["topped_up_balance"]),
			Enough:   enough,
		}),
	}, nil
}

func fetchOllama(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey("ollama-cloud", []string{"OLLAMA_API_KEY"}, "Ollama Cloud")
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, ollamaURL, bearer(key), readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something this plugin could not read."}
	}
	included, _ := asObject(answer["included"])
	purchased, _ := asObject(answer["purchased"])
	allowance := percentOf(included["allowance_usd"])
	granted := percentOf(included["balance_usd"])
	toppedUp := percentOf(purchased["balance_usd"])
	var limits []Limit
	if allowance > 0 {
		percent := min(100, max(0, (allowance-granted)/allowance*100))
		period, _ := asObject(included["period"])
		start := resetAt(period["from"])
		end := resetAt(period["until"])
		var minutes float64
		if !start.IsZero() && !end.IsZero() {
			minutes = end.Sub(start).Minutes()
		}
		limits = append(limits, Limit{
			Label:    "Month",
			Percent:  percent,
			ResetsAt: end,
			Level:    LevelOf(percent, ""),
			Window:   WindowDuration(minutes),
		})
	}
	total := granted + toppedUp
	return Report{
		Name:   ollamaName,
		Limits: limits,
		Notes:  []string{OffPeakNote(time.Now().UTC())},
		Balance: new(Balance{
			Currency: "USD",
			Total:    total,
			Granted:  granted,
			ToppedUp: toppedUp,
			Enough:   total > 0,
		}),
	}, nil
}

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

func fetchMiniMax(ctx context.Context, source Source, client *http.Client) (Report, error) {
	token, err := source.MiniMaxToken()
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

func fetchMuse(ctx context.Context, source Source, _ *http.Client) (Report, error) {
	command, err := source.Command("QUOTAS_MUSE_COMMAND", "muse", museMissing)
	if err != nil {
		return Report{}, err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, readWait, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))})
	defer cancel()
	process := exec.CommandContext(ctx, command, "usage", "--json")
	process.Dir = source.userHome()
	output, err := process.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Report{}, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))}
		}
		return Report{}, MissingError{Text: museMissing}
	}
	var decoded any
	if err := json.Unmarshal(output, &decoded); err != nil {
		return Report{}, MissingError{Text: "The Muse CLI did not answer with its usage: run muse login, then try again."}
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, MissingError{Text: "The Muse CLI did not answer with its usage: run muse login, then try again."}
	}
	allowance, ok := asObject(answer["allowance"])
	if !ok {
		return Report{Name: museName, Notes: []string{"No usage observed yet: Muse reports it after the first use in a window."}}, nil
	}
	percent := percentOf(allowance["usedPercent"])
	duration, durationOK := asNumber(allowance["windowDurationMins"])
	label := "5 hours"
	minutes := 300.0
	if durationOK {
		label = WindowLabel(duration)
		minutes = duration
	}
	limits := []Limit{{
		Label:    label,
		Percent:  percent,
		ResetsAt: resetAtMillis(allowance["resetsAtMs"]),
		Level:    LevelOf(percent, ""),
		Window:   WindowDuration(minutes),
	}}
	if weekly, ok := asObject(allowance["weekly"]); ok {
		weekPercent := percentOf(weekly["usedPercent"])
		limits = append(limits, Limit{
			Label:    "Week",
			Percent:  weekPercent,
			ResetsAt: resetAtMillis(weekly["resetsAtMs"]),
			Level:    LevelOf(weekPercent, ""),
			Window:   weekWindow,
		})
	}
	var notes []string
	if tier, ok := asText(allowance["tier"]); ok {
		notes = append(notes, capitalize(Spaced(tier))+" plan.")
	}
	return Report{Name: museName, Limits: limits, Notes: notes}, nil
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func firstRecord(records []map[string]any) map[string]any {
	if len(records) == 0 {
		return nil
	}
	return records[0]
}

func aboveZero(number float64) bool { return number > 0 }

func wholeNumber(number float64) bool { return number == float64(int(number)) }

func monthUnit(unit float64) bool { return int(unit) == 5 }

func refusedCode(code float64) bool { return code != 0 }

func emptyCount(total float64) bool { return total <= 0 }

func unusedWeek(left float64) bool { return left >= 100 }

func weekLabel(label string) bool { return label == "Week" }

func missing(ok bool) bool { return !ok }

func ready(ok bool) bool { return ok }

func wholePair(unit, count any) (float64, float64, bool) {
	unitNumber, unitOK := asNumber(unit)
	countNumber, countOK := asNumber(count)
	if !unitOK || missing(countOK) || !wholeNumber(unitNumber) || !wholeNumber(countNumber) || !aboveZero(countNumber) {
		return 0, 0, false
	}
	return unitNumber, countNumber, true
}

func deepSeekEnough(answer map[string]any, total float64) bool {
	if asBool(answer["is_available"]) {
		return true
	}
	if answer["is_available"] == false {
		return false
	}
	return aboveZero(total)
}
