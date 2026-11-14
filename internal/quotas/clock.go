package quotas

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	gaugeCells = 20
	week       = 7 * 24 * time.Hour
)

// Lit is how many of a gauge's squares are filled.
// Any use lights at least one, and the gauge is not full until the limit is.
func Lit(percent float64) int {
	if percent <= 0 {
		return 0
	}
	count := max(1, int(math.Round(percent/5)))
	if percent < 100 {
		return min(count, gaugeCells-1)
	}
	return gaugeCells
}

// WindowShare is how far a limit is into its window, in percent.
// The second result is false when the service gives no clock.
func WindowShare(limit Limit, now time.Time) (int, bool) {
	share, ok := shareOf(limit, now)
	if !ok {
		return 0, false
	}
	return int(math.Round(share)), true
}

// Lag is how far a limit is behind its window: the share that has passed,
// and the points use is behind that share.
func Lag(limit Limit, now time.Time) (share int, points float64, ok bool) {
	share, ok = WindowShare(limit, now)
	if !ok {
		return 0, 0, false
	}
	return share, float64(share) - limit.Percent, true
}

// UntilReset is "in 2d 4h", "in 3h 10m", "in 12m", or "now".
func UntilReset(reset time.Time, now time.Time) (string, bool) {
	if reset.IsZero() {
		return "", false
	}
	minutes := max(0, int(math.Round(reset.Sub(now).Minutes())))
	if minutes == 0 {
		return "now", true
	}
	days := minutes / 1440
	hours := (minutes % 1440) / 60
	rest := minutes % 60
	switch {
	case days > 0 && hasHours(hours):
		return fmt.Sprintf("in %dd %dh", days, hours), true
	case days > 0:
		return fmt.Sprintf("in %dd", days), true
	case hours > 0 && hasMinutes(rest):
		return fmt.Sprintf("in %dh %dm", hours, rest), true
	case hours > 0:
		return fmt.Sprintf("in %dh", hours), true
	default:
		return fmt.Sprintf("in %dm", minutes), true
	}
}

// ResetClock is "Mon 4:08 AM" for a reset this week, and "Nov 5, 2:08 AM" further away.
func ResetClock(reset time.Time, now time.Time) string {
	if reset.IsZero() {
		return ""
	}
	local := reset.Local()
	if reset.Sub(now) <= week {
		return local.Format("Mon 3:04 PM")
	}
	return local.Format("Jan 2, 3:04 PM")
}

// ResetLabel is the clock text the gauge shows: the wait, without a leading "in ".
func ResetLabel(reset time.Time, now time.Time) (string, bool) {
	label, ok := UntilReset(reset, now)
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(label, "in "), true
}

// Money formats an amount the way a narrow currency symbol would, or the code
// and the number when the code is not one of the common ones.
func Money(value float64, currency string) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}
	if currency == "" {
		currency = "CNY"
	}
	symbols := map[string]string{"USD": "$", "CNY": "¥", "EUR": "€", "GBP": "£"}
	if symbol, ok := symbols[currency]; ok {
		return fmt.Sprintf("%s%.2f", symbol, value)
	}
	return fmt.Sprintf("%s %.2f", currency, value)
}

// Age is how old a reading is: "<1m ago", "3m ago", "2h ago", or "2d ago".
func Age(at time.Time, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	seconds := max(0, int(math.Round(now.Sub(at).Seconds())))
	if seconds < 60 {
		return "<1m ago"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", hours/24)
}

// WindowLabel is a window's length as people read it. 300 minutes is "5 hours".
func WindowLabel(minutes float64) string {
	if math.IsNaN(minutes) || math.IsInf(minutes, 0) || nonPositive(minutes) {
		return "Limit"
	}
	if minutes == 10_080 {
		return "Week"
	}
	if int(minutes)%1440 == 0 {
		days := int(minutes) / 1440
		if days == 1 {
			return "Day"
		}
		return fmt.Sprintf("%d days", days)
	}
	if int(minutes)%60 == 0 {
		return fmt.Sprintf("%d hours", int(minutes)/60)
	}
	return fmt.Sprintf("%d minutes", int(minutes))
}

// WindowDuration is a limit's window from the minutes a service gives.
func WindowDuration(minutes float64) time.Duration {
	if math.IsNaN(minutes) || math.IsInf(minutes, 0) || nonPositive(minutes) {
		return 0
	}
	return time.Duration(int(minutes*60_000)) * time.Millisecond
}

// LevelOf is a level from the service's own word, else from how full the limit is.
func LevelOf(percent float64, said string) Level {
	switch said {
	case "critical", "exceeded", "limited", "blocked":
		return LevelCritical
	case "warning":
		return LevelWarning
	}
	if percent >= 90 {
		return LevelCritical
	}
	if percent >= 75 {
		return LevelWarning
	}
	return LevelOK
}

// OffPeakNote says which rates Ollama Cloud charges now, and when that changes.
// Off-peak is outside 12:00 to 18:00 UTC on weekdays, and all weekend.
func OffPeakNote(now time.Time) string {
	utc := now.UTC()
	full := peakPrice(utc)
	until := utc.Truncate(time.Hour)
	for range 8 * 24 {
		until = until.Add(time.Hour)
		if peakPrice(until) != full {
			break
		}
	}
	local := until.Local()
	clock := local.Format("3:04 PM MST")
	day := ""
	if local.YearDay() != now.Local().YearDay() || differentYear(local, now.Local()) {
		day = local.Format("Mon ")
	}
	if !full {
		return fmt.Sprintf("Off-peak rates until %s%s.", day, clock)
	}
	return fmt.Sprintf("Full price until %s%s: off-peak rates after.", day, clock)
}

func peakPrice(at time.Time) bool {
	if at.Weekday() == time.Saturday {
		return false
	}
	if at.Weekday() == time.Sunday {
		return false
	}
	if at.Hour() < 12 {
		return false
	}
	return at.Hour() < 18
}

func hasHours(hours int) bool { return hours > 0 }

func hasMinutes(minutes int) bool { return minutes > 0 }

func nonPositive(minutes float64) bool { return minutes <= 0 }

func differentYear(local, now time.Time) bool { return local.Year() != now.Year() }

func capitalize(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
