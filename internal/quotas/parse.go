package quotas

import (
	"math"
	"strconv"
	"strings"
	"time"
)

func asObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func asRecords(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if object, ok := asObject(item); ok {
			records = append(records, object)
		}
	}
	return records
}

func asText(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && hasText(text)
}

func asNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case bool:
		return 0, false
	case float64:
		return finite(value)
	case float32:
		return finite(float64(value))
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint64:
		return float64(value), true
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, false
		}
		return finite(number)
	default:
		return 0, false
	}
}

func finite(number float64) (float64, bool) {
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func percentOf(value any) float64 {
	number, ok := asNumber(value)
	if !ok {
		return 0
	}
	return max(0, number)
}

func asBool(value any) bool {
	flag, ok := value.(bool)
	if !ok {
		return false
	}
	return flag
}

// resetAt reads an ISO time or seconds since 1970. Zero and blank mean no clock.
func resetAt(value any) time.Time {
	switch value := value.(type) {
	case bool:
		return time.Time{}
	case string:
		return parseTime(value)
	default:
		number, ok := asNumber(value)
		if !ok || unsetClock(number) || clockTooFar(number) {
			return time.Time{}
		}
		seconds, fraction := math.Modf(number)
		return time.Unix(int64(seconds), int64(fraction*float64(time.Second))).UTC()
	}
}

func resetAtMillis(value any) time.Time {
	if _, isBool := value.(bool); isBool {
		return time.Time{}
	}
	number, ok := asNumber(value)
	if !ok {
		return time.Time{}
	}
	return resetAt(number / 1000)
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err != nil {
			continue
		}
		if parsed.Location() == time.UTC || parsed.Location() == time.Local && strings.HasSuffix(value, "Z") {
			return parsed.UTC()
		}
		return parsed.UTC()
	}
	return time.Time{}
}

func firstRecord(records []map[string]any) map[string]any {
	if len(records) == 0 {
		return nil
	}
	return records[0]
}

func aboveZero(number float64) bool { return number > 0 }

func wholeNumber(number float64) bool { return number == float64(int(number)) }

func missing(ok bool) bool { return !ok }

func ready(ok bool) bool { return ok }

func hasText(text string) bool { return text != "" }

func unsetClock(number float64) bool { return number == 0 }

func clockTooFar(number float64) bool { return math.Abs(number) > 8.64e12 }
