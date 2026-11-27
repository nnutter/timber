package quotas

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	nameStyle     = lipgloss.NewStyle().Bold(true)
	percentStyle  = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Faint(true)
	quietStyle    = lipgloss.NewStyle().Faint(true)
	criticalStyle = lipgloss.NewStyle().Foreground(ansi.Red)
	ruleStyle     = lipgloss.NewStyle().Faint(true)
	trackStyle    = lipgloss.NewStyle().Foreground(ansi.BrightBlack)
)

const (
	paceOKPoints      = 10
	paceWarningPoints = 20
	// minWidth is the narrowest report that still fits a name and a percent.
	minWidth = 24
	// maxWidth keeps the name, the reset, and the percent on one line.
	// Wider than this, the reset would drift away from both.
	maxWidth = 60

	usageFull    = '█'
	usageEmpty   = '░'
	elapsedFull  = '━'
	elapsedEmpty = '─'
	// elapsedTip sits on the last spent cell and turns up toward the usage bar.
	elapsedTip = '┛'
)

// Render draws the plans in the order to use them.
// width is the terminal width. The report stays between a narrow default and 60 columns.
func Render(snapshot Snapshot, now time.Time, width int) string {
	width = frameWidth(width)
	reports := Ordered(snapshot.Reports, now)
	if len(reports) == 0 {
		return mutedStyle.Render("Nothing to report yet: sign in to a provider in Hermes.")
	}
	var out strings.Builder
	for index, report := range reports {
		if index > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(renderPlan(report, now, width))
	}
	return out.String()
}

func renderPlan(report Report, now time.Time, width int) string {
	var out strings.Builder
	out.WriteString(renderHead(report, width))
	if !report.OK {
		out.WriteString("\n")
		out.WriteString(mutedStyle.Render(report.Error))
		return out.String()
	}
	if report.Balance != nil && showsBalance(report) {
		out.WriteString("\n")
		out.WriteString(renderBalance(*report.Balance))
	}
	showNotes := len(report.Limits) == 0
	for index, limit := range report.Limits {
		if limit.Percent < exhaustedPercent {
			showNotes = true
		}
		if index > 0 {
			out.WriteString("\n")
		}
		out.WriteString("\n")
		out.WriteString(renderLimit(report, limit, now, width))
	}
	if showNotes {
		for _, note := range report.Notes {
			out.WriteString("\n")
			out.WriteString(mutedStyle.Render(note))
		}
	}
	return out.String()
}

func renderHead(report Report, width int) string {
	name := nameStyle.Render(report.Name)
	if report.OK {
		ruleWidth := width - ansi.StringWidth(report.Name) - 1
		return name + " " + ruleStyle.Render(strings.Repeat("─", max(ruleWidth, 1)))
	}
	const status = "Unavailable"
	ruleWidth := width - ansi.StringWidth(report.Name) - ansi.StringWidth(status) - 2
	rule := ruleStyle.Render(strings.Repeat("─", max(ruleWidth, 1)))
	return name + " " + rule + " " + criticalStyle.Render(status)
}

func renderLimit(report Report, limit Limit, now time.Time, width int) string {
	if limit.Percent >= exhaustedPercent {
		if reset := ResetClock(limit.ResetsAt, now); reset != "" {
			return limit.Label + " " + quietStyle.Render("Resets "+reset)
		}
		return limit.Label + " " + quietStyle.Render("Reset time unavailable")
	}
	label := limitLabel(report, limit)
	percentPlain := fmt.Sprintf("%d%%", int(math.Round(limit.Percent)))
	middle := limitMiddle(limit, now, width-ansi.StringWidth(label)-ansi.StringWidth(percentPlain))
	elapsed, hasClock := shareOf(limit, now)
	color := levelColor(limit.Level)
	if hasClock {
		color = paceColor(limit.Percent - elapsed)
	}
	fill := lipgloss.NewStyle().Foreground(color)

	var out strings.Builder
	out.WriteString(placeWindowLine(label, middle, percentPlain, width))
	out.WriteString("\n")
	out.WriteString(renderGauge(limit.Percent, width, fill))
	if hasClock {
		out.WriteString("\n")
		out.WriteString(renderPace(elapsed, width, fill))
	}
	if middle == "" {
		if meta := renderMeta(limit, now); meta != "" {
			out.WriteString("\n")
			out.WriteString(meta)
		}
	}
	return out.String()
}

func limitMiddle(limit Limit, now time.Time, room int) string {
	withClock := windowClock(limit, now)
	if fitsMiddle(withClock, room) {
		return withClock
	}
	reset := windowReset(limit, now)
	if fitsMiddle(reset, room) {
		return reset
	}
	return ""
}

func fitsMiddle(text string, room int) bool {
	if text == "" {
		return false
	}
	return ansi.StringWidth(text)+2 <= room
}

func placeWindowLine(label, middle, percentPlain string, width int) string {
	percent := stylePercent(percentPlain)
	if middle == "" {
		return spread(label, percent, percentPlain, width)
	}
	return centerOnLine(label, quietStyle.Render(middle), middle, percent, percentPlain, width)
}

func stylePercent(percentPlain string) string {
	number, found := strings.CutSuffix(percentPlain, "%")
	if !found {
		return percentStyle.Render(percentPlain)
	}
	return percentStyle.Render(number) + mutedStyle.Render("%")
}

func spread(label, percent, percentPlain string, width int) string {
	gap := width - ansi.StringWidth(label) - ansi.StringWidth(percentPlain)
	if gap < 1 {
		return label + " " + percent
	}
	return label + strings.Repeat(" ", gap) + percent
}

// centerOnLine puts the reset on the center of the line.
// Equal space on either side would slide it toward the percent, because the
// name is wider than the percent.
func centerOnLine(label, middle, middlePlain, percent, percentPlain string, width int) string {
	labelWidth := ansi.StringWidth(label)
	middleWidth := ansi.StringWidth(middlePlain)
	percentWidth := ansi.StringWidth(percentPlain)
	if !roomForReset(labelWidth, middleWidth, percentWidth, width) {
		return spread(label, percent, percentPlain, width)
	}
	start := clampedStart((width-middleWidth)/2, labelWidth+1, width-percentWidth-middleWidth-1)
	leftPad := start - labelWidth
	rightPad := width - percentWidth - (start + middleWidth)
	return label + strings.Repeat(" ", leftPad) + middle + strings.Repeat(" ", rightPad) + percent
}

func roomForReset(labelWidth, middleWidth, percentWidth, width int) bool {
	return width-labelWidth-middleWidth-percentWidth >= 2
}

func clampedStart(start, low, high int) int {
	if start < low {
		return low
	}
	if start > high {
		return high
	}
	return start
}

func windowReset(limit Limit, now time.Time) string {
	var parts []string
	if limit.Binding {
		parts = append(parts, "In force")
	}
	if reset, ok := ResetLabel(limit.ResetsAt, now); ok {
		parts = append(parts, reset)
	}
	return strings.Join(parts, " · ")
}

func windowClock(limit Limit, now time.Time) string {
	text := windowReset(limit, now)
	if text == "" {
		return ""
	}
	if _, ok := ResetLabel(limit.ResetsAt, now); !ok {
		return text
	}
	clock := ResetClock(limit.ResetsAt, now)
	if clock == "" {
		return text
	}
	return text + "  " + clock
}

func renderGauge(percent float64, width int, fill lipgloss.Style) string {
	filled := filledCells(width, percent)
	used := fill.Render(strings.Repeat(string(usageFull), filled))
	remaining := trackStyle.Render(strings.Repeat(string(usageEmpty), width-filled))
	return used + remaining
}

// renderPace draws how far the window has run, on the same width as the usage bar.
// An arrow at the tip of the time already spent points up at the usage.
func renderPace(elapsed float64, width int, fill lipgloss.Style) string {
	filled := max(1, filledCells(width, elapsed))
	spent := fill.Render(strings.Repeat(string(elapsedFull), filled-1) + string(elapsedTip))
	ahead := trackStyle.Render(strings.Repeat(string(elapsedEmpty), width-filled))
	return spent + ahead
}

func filledCells(width int, percent float64) int {
	portion := min(max(percent, 0), 100) / 100
	return min(max(int(math.Round(float64(width)*portion)), 0), width)
}

func levelColor(level Level) ansi.BasicColor {
	switch level {
	case LevelCritical:
		return ansi.Red
	case LevelWarning:
		return ansi.Yellow
	default:
		return ansi.Green
	}
}

func paceColor(pointsAhead float64) ansi.BasicColor {
	gap := math.Abs(pointsAhead)
	switch {
	case gap > paceWarningPoints:
		return ansi.Red
	case gap > paceOKPoints:
		return ansi.Yellow
	case pointsAhead > 0:
		return ansi.Blue
	default:
		return ansi.Green
	}
}

func renderMeta(limit Limit, now time.Time) string {
	text := windowReset(limit, now)
	if text == "" {
		return ""
	}
	return quietStyle.Render(text)
}

func renderBalance(balance Balance) string {
	var out strings.Builder
	out.WriteString(percentStyle.Render(Money(balance.Total, balance.Currency)))
	if !balance.Enough {
		out.WriteString(" ")
		out.WriteString(criticalStyle.Render("Not enough for calls"))
	}
	var parts []string
	if balance.Granted > 0 && separateGrant(balance) {
		parts = append(parts, "Granted "+Money(balance.Granted, balance.Currency))
	}
	if balance.ToppedUp > 0 && separateTopUp(balance) {
		parts = append(parts, "Topped up "+Money(balance.ToppedUp, balance.Currency))
	}
	if len(parts) > 0 {
		out.WriteString("\n")
		out.WriteString(mutedStyle.Render(strings.Join(parts, " · ")))
	}
	return out.String()
}

func limitLabel(report Report, limit Limit) string {
	switch {
	case report.ID == "ollama" && hasBalance(report):
		return fmt.Sprintf("%s (%s)", limit.Label, Money(report.Balance.Granted, report.Balance.Currency))
	case report.ID == "github-copilot" && hasCredits(limit):
		return fmt.Sprintf("%s (%s)", limit.Label, Money(limit.RemainingCredits/100, "USD"))
	default:
		return limit.Label
	}
}

func showsBalance(report Report) bool {
	if report.Balance == nil {
		return false
	}
	if report.ID != "ollama" {
		return true
	}
	return len(report.Limits) == 0 || toppedUp(report.Balance) || notEnough(report.Balance)
}

func frameWidth(width int) int {
	if width < minWidth {
		return minWidth
	}
	if width > maxWidth {
		return maxWidth
	}
	return width
}

func separateGrant(balance Balance) bool { return balance.Granted != balance.Total }

func separateTopUp(balance Balance) bool { return balance.ToppedUp != balance.Total }

func hasBalance(report Report) bool { return report.Balance != nil }

func hasCredits(limit Limit) bool { return limit.HasRemainingCredits }

func toppedUp(balance *Balance) bool { return balance.ToppedUp > 0 }

func notEnough(balance *Balance) bool { return !balance.Enough }
