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
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	criticalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	ruleStyle     = lipgloss.NewStyle().Faint(true)
)

const (
	lagPoints = 5
	lagFar    = 15
	// minWidth is the narrowest report that still fits a name and a percent.
	minWidth = 24
	// maxWidth keeps the percent next to its plan. A wide terminal would
	// otherwise push it to the far edge.
	maxWidth = 72
)

// Render draws the plans in the order to use them.
// width is the terminal width. The report stays between a narrow default and 72 columns.
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
	for _, limit := range report.Limits {
		out.WriteString("\n")
		out.WriteString(renderLimit(report, limit, now, width))
	}
	for _, note := range report.Notes {
		out.WriteString("\n")
		out.WriteString(mutedStyle.Render(note))
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
	label := limitLabel(report, limit)
	percent := int(math.Round(limit.Percent))
	percentText := percentStyle.Render(fmt.Sprintf("%d", percent)) + mutedStyle.Render("%")
	gap := width - ansi.StringWidth(label) - ansi.StringWidth(fmt.Sprintf("%d%%", percent))
	var row string
	if gap >= 1 {
		row = label + strings.Repeat(" ", gap) + percentText
	} else {
		row = label + " " + percentText
	}

	var out strings.Builder
	out.WriteString(row)
	out.WriteString("\n")
	out.WriteString(renderGauge(limit))
	if share, points, ok := Lag(limit, now); ok {
		out.WriteString("\n")
		out.WriteString(renderPace(share, points))
	}
	if meta := renderMeta(limit, now); meta != "" {
		out.WriteString("\n")
		out.WriteString(meta)
	}
	return out.String()
}

func renderGauge(limit Limit) string {
	on := Lit(limit.Percent)
	return tone(limit.Level).Render(strings.Repeat("■", on)) + mutedStyle.Render(strings.Repeat("□", gaugeCells-on))
}

func renderPace(share int, points float64) string {
	filled := int(math.Round(float64(gaugeCells) * float64(share) / 100))
	filled = min(max(filled, 0), gaugeCells)
	gap := int(math.Round(points))
	style := mutedStyle
	switch {
	case gap >= lagFar:
		style = criticalStyle
	case gap >= lagPoints:
		style = warningStyle
	}
	return style.Render(strings.Repeat("━", filled)) + mutedStyle.Render(strings.Repeat("━", gaugeCells-filled))
}

func renderMeta(limit Limit, now time.Time) string {
	reset, hasReset := ResetLabel(limit.ResetsAt, now)
	if !limit.Binding && noReset(hasReset) {
		return ""
	}
	var parts []string
	if limit.Binding {
		parts = append(parts, "In force")
	}
	if hasReset {
		parts = append(parts, reset)
	}
	text := strings.Join(parts, " · ")
	if hasReset {
		if clock := ResetClock(limit.ResetsAt, now); clock != "" {
			text += "  " + clock
		}
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

func tone(level Level) lipgloss.Style {
	switch level {
	case LevelCritical:
		return criticalStyle
	case LevelWarning:
		return warningStyle
	default:
		return okStyle
	}
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

func noReset(hasReset bool) bool { return !hasReset }

func separateGrant(balance Balance) bool { return balance.Granted != balance.Total }

func separateTopUp(balance Balance) bool { return balance.ToppedUp != balance.Total }

func hasBalance(report Report) bool { return report.Balance != nil }

func hasCredits(limit Limit) bool { return limit.HasRemainingCredits }

func toppedUp(balance *Balance) bool { return balance.ToppedUp > 0 }

func notEnough(balance *Balance) bool { return !balance.Enough }
