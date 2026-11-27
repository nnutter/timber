package quotas

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderKeepsThePercentNextToThePlan(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	view := Render(Snapshot{
		Reports: []Report{plan("codex", "ChatGPT Plus", timed("Week", 22, 7*24*time.Hour, 0.5, now))},
	}, now, 200)

	for line := range strings.SplitSeq(view, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 60)
	}
	assert.Equal(t, 60, ansi.StringWidth(strings.Split(view, "\n")[0]))
}

func TestRenderShowsOnlyTheResetForExhaustedQuota(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(8 * 24 * time.Hour)
	report := plan("example", "Example", Limit{
		Label: "Month", Percent: 100, Binding: true, Window: 30 * 24 * time.Hour, ResetsAt: reset,
	})
	report.Notes = []string{"A note about this plan."}
	view := ansi.Strip(Render(Snapshot{Reports: []Report{report}}, now, 60))

	lines := strings.Split(view, "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "Month Resets "+reset.Local().Format("Jan 2, 3:04 PM"), lines[1])
}

func TestRenderCentersTheResetOnTheWindowLine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	month := 30 * 24 * time.Hour
	view := ansi.Strip(Render(Snapshot{Reports: []Report{{
		ID: "ollama", Name: "Ollama Cloud", OK: true,
		Balance: &Balance{Currency: "USD", Granted: 6.21, Total: 6.21, Enough: true},
		Limits:  []Limit{timed("Month", 90, month, 0.42, now)},
	}}}, now, 200))

	line := lineContaining(view, "Month ($6.21)")
	percentAt := strings.LastIndex(line, "90%")
	require.NotEqual(t, -1, percentAt)
	name := "Month ($6.21)"
	nameEnd := strings.Index(line, name) + len(name)
	middle := strings.TrimSpace(line[nameEnd:percentAt])
	require.NotEmpty(t, middle)
	middleAt := strings.Index(line, middle)
	require.NotEqual(t, -1, middleAt)
	center := middleAt + len(middle)/2
	assert.InDelta(t, float64(len(line)/2), float64(center), 1)
	assert.Equal(t, 60, ansi.StringWidth(line))
}

func lineContaining(view, mark string) string {
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, mark) {
			return line
		}
	}
	return ""
}

func TestRenderShowsThePlanToUseFirst(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	view := ansi.Strip(Render(Snapshot{
		FetchedAt: now.Add(-time.Minute),
		Reports: []Report{
			plan("codex", "ChatGPT Plus", timed("Week", 22, week, 0.525, now)),
			plan("muse", "Muse Code", timed("Week", 36, week, 0.225, now)),
		},
	}, now, 60))

	muse := strings.Index(view, "Muse Code")
	chatgpt := strings.Index(view, "ChatGPT Plus")
	assert.GreaterOrEqual(t, muse, 0)
	assert.Greater(t, chatgpt, muse)
	assert.Contains(t, view, "36%")
	assert.Contains(t, view, "Week")
	assert.NotContains(t, view, "1m ago")
	assert.NotContains(t, view, "<1m ago")
	assert.Contains(t, view, "█")
	assert.Contains(t, view, "░")
}

func TestRenderShowsHowMuchOfEachWindowHasPassed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	month := 30 * 24 * time.Hour
	week := 7 * 24 * time.Hour
	view := ansi.Strip(Render(Snapshot{Reports: []Report{plan("opencode-go", "OpenCode Go",
		Limit{Label: "Month", Percent: 10, Level: LevelOK, Window: month, ResetsAt: now.Add(month / 4)},
		Limit{Label: "Week", Percent: 10, Level: LevelOK, Window: week, ResetsAt: now.Add(week / 2)},
	)}}, now, 40))

	assert.Contains(t, view, "\n\nWeek")
	elapsed := linesContaining(view, "━")
	require.Len(t, elapsed, 2)
	assert.Equal(t, 29, strings.Count(elapsed[0], "━"))
	assert.Equal(t, 1, strings.Count(elapsed[0], "┛"))
	assert.Equal(t, 10, strings.Count(elapsed[0], "─"))
	assert.Contains(t, elapsed[0], "━┛─")
	assert.Equal(t, 19, strings.Count(elapsed[1], "━"))
	assert.Equal(t, 1, strings.Count(elapsed[1], "┛"))
	assert.Equal(t, 20, strings.Count(elapsed[1], "─"))
	assert.Contains(t, elapsed[1], "━┛─")
	assert.Equal(t, 40, ansi.StringWidth(elapsed[0]))
	assert.Equal(t, 40, ansi.StringWidth(elapsed[1]))
}

func TestRenderUsesTheSamePaceColorForUsageAndElapsed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	cases := []struct {
		name    string
		percent float64
		color   string
	}{
		{"10 points below", 40, "\x1b[32m"},
		{"exactly on pace", 50, "\x1b[32m"},
		{"just above pace", 50.1, "\x1b[34m"},
		{"10 points above", 60, "\x1b[34m"},
		{"just over 10 points below", 39.9, "\x1b[33m"},
		{"just over 10 points above", 60.1, "\x1b[33m"},
		{"20 points below", 30, "\x1b[33m"},
		{"20 points above", 70, "\x1b[33m"},
		{"just over 20 points below", 29.9, "\x1b[31m"},
		{"just over 20 points above", 70.1, "\x1b[31m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit := Limit{Label: "Week", Percent: tc.percent, Level: LevelCritical, Window: week, ResetsAt: now.Add(week / 2)}
			view := Render(Snapshot{Reports: []Report{plan("opencode-go", "OpenCode Go", limit)}}, now, 40)
			gauge := lineContaining(view, "█")
			elapsed := lineContaining(view, "━")
			assert.Contains(t, gauge, tc.color+"█")
			assert.Contains(t, elapsed, tc.color+"━")
			assert.Contains(t, gauge, "\x1b[90m░")
			assert.Contains(t, elapsed, "\x1b[90m─")
		})
	}
}

func TestRenderUsesLimitStatusWhenThereIsNoClock(t *testing.T) {
	t.Parallel()
	limit := Limit{Label: "Credits", Percent: 12, Level: LevelWarning}
	view := Render(Snapshot{Reports: []Report{plan("example", "Example", limit)}}, time.Time{}, 40)
	assert.Contains(t, lineContaining(view, "█"), "\x1b[33m█")
	assert.NotContains(t, view, "┛")
}

func linesContaining(view, mark string) []string {
	var found []string
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, mark) {
			found = append(found, line)
		}
	}
	return found
}

func TestRenderLeavesOutAMissingSignIn(t *testing.T) {
	t.Parallel()
	view := ansi.Strip(Render(Snapshot{Reports: []Report{{
		ID: "zai", Name: "Z.ai", Missing: true, Error: "Add a Z.ai key in Hermes to read its plan.",
	}}}, time.Now(), 40))

	assert.Contains(t, view, "Nothing to report yet")
	assert.NotContains(t, view, "Z.ai")
}

func TestRenderSaysWhyAServiceCouldNotBeRead(t *testing.T) {
	t.Parallel()
	view := ansi.Strip(Render(Snapshot{Reports: []Report{{
		ID: "zai", Name: "Z.ai", Error: "It answered 500.",
	}}}, time.Now(), 40))

	assert.Contains(t, view, "Z.ai")
	assert.Contains(t, view, "Unavailable")
	assert.Contains(t, view, "It answered 500.")
}

func TestRenderShowsABalanceAndCopilotCredits(t *testing.T) {
	t.Parallel()
	now := time.Now()
	view := ansi.Strip(Render(Snapshot{Reports: []Report{
		{
			ID:   "deepseek",
			Name: "DeepSeek",
			OK:   true,
			Balance: new(Balance{
				Currency: "CNY", Total: 8.5, Granted: 3, ToppedUp: 5.5, Enough: false,
			}),
		},
		{
			ID:   "github-copilot",
			Name: "GitHub Copilot",
			OK:   true,
			Limits: []Limit{{
				Label: "Month", Percent: 40, Level: LevelOK,
				HasRemainingCredits: true, RemainingCredits: 250,
			}},
		},
	}}, now, 60))

	assert.Contains(t, view, "¥8.50")
	assert.Contains(t, view, "Not enough for calls")
	assert.Contains(t, view, "Granted ¥3.00")
	assert.Contains(t, view, "Topped up ¥5.50")
	assert.Contains(t, view, "Month ($2.50)")
}
