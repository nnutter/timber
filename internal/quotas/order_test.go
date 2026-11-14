package quotas

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOrderedPutsTheMostWasteFirst(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	month := 30 * 24 * time.Hour
	reports := []Report{
		plan("ollama", "Ollama Cloud", timed("Month", 70, month, 0.4667, now)),
		plan("opencode-go", "OpenCode Go", timed("Month", 50, month, 0.5, now)),
		plan("codex", "ChatGPT Plus", timed("Week", 22, week, 0.525, now)),
		plan("muse", "Muse Code", timed("Week", 36, week, 0.225, now)),
		plan("xai", "SuperGrok", timed("Week", 5, week, 0.1, now)),
	}

	assert.Equal(t, []string{"SuperGrok", "Muse Code", "ChatGPT Plus", "OpenCode Go", "Ollama Cloud"}, names(Ordered(reports, now)))
}

func TestOrderedPutsUntimedPlansAfterTimedOnes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	reports := []Report{
		{ID: "xai", Name: "Grok", Error: "It answered 500."},
		plan("opencode-go", "OpenCode Go", Limit{Label: "Month", Percent: 83, Level: LevelWarning}),
		plan("anthropic", "Claude (Anthropic)", Limit{Label: "Week", Percent: 32, Level: LevelOK}),
	}

	assert.Equal(t, []string{"Claude (Anthropic)", "OpenCode Go", "Grok"}, names(Ordered(reports, now)))
}

func TestOrderedKeepsAPlanItCannotTimeAfterThePlansItCan(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	reports := []Report{
		plan("zai", "Z.ai", Limit{Label: "Week", Percent: 80, Level: LevelWarning}),
		{
			ID:      "deepseek",
			Name:    "DeepSeek",
			OK:      true,
			Balance: new(Balance{Currency: "USD", Total: 8.5, Granted: 3, ToppedUp: 5.5, Enough: true}),
		},
		plan("muse", "Muse Code", timed("Week", 40, week, 0.5, now)),
	}

	assert.Equal(t, []string{"Muse Code", "Z.ai", "DeepSeek"}, names(Ordered(reports, now)))
}

func TestOrderedPutsAnExhaustedWindowWithQuotaLeftFirst(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	week := 7 * 24 * time.Hour
	reports := []Report{
		plan("codex", "ChatGPT Plus", timed("Week", 90, week, 0.01, now)),
		plan("muse", "Muse Code", Limit{
			Label:    "Week",
			Percent:  40,
			Level:    LevelOK,
			Window:   week,
			ResetsAt: now.Add(-time.Hour),
		}),
	}

	assert.Equal(t, []string{"Muse Code", "ChatGPT Plus"}, names(Ordered(reports, now)))
}

func TestOrderedLeavesOutAMissingSignIn(t *testing.T) {
	t.Parallel()
	reports := []Report{{
		ID:      "zai",
		Name:    "Z.ai",
		Missing: true,
		Error:   "Add a Z.ai key in Hermes to read its plan.",
	}}

	assert.Empty(t, Ordered(reports, time.Now()))
}

func TestStandingDropsAShorterLimitWhenALongerOneIsUsedUp(t *testing.T) {
	t.Parallel()
	week := 7 * 24 * time.Hour
	session := 5 * time.Hour
	limits := Standing([]Limit{
		{Label: "5-hour session", Percent: 40, Window: session},
		{Label: "Week, all models", Percent: 100, Window: week},
		{Label: "Unknown", Percent: 10},
	})

	assert.Equal(t, []string{"Week, all models", "Unknown"}, labels(limits))
}

func TestStandingKeepsAShorterLimitWhenTheLongerOneStillHasRoom(t *testing.T) {
	t.Parallel()
	limits := Standing([]Limit{
		{Label: "5-hour session", Percent: 40, Window: 5 * time.Hour},
		{Label: "Week, all models", Percent: 80, Window: 7 * 24 * time.Hour},
	})

	assert.Equal(t, []string{"5-hour session", "Week, all models"}, labels(limits))
}

func plan(id, name string, limits ...Limit) Report {
	return Report{ID: id, Name: name, OK: true, Limits: limits}
}

func timed(label string, percent float64, window time.Duration, left float64, now time.Time) Limit {
	return Limit{
		Label:    label,
		Percent:  percent,
		Level:    LevelOK,
		Window:   window,
		ResetsAt: now.Add(time.Duration(float64(window) * left)),
	}
}

func names(reports []Report) []string {
	got := make([]string, 0, len(reports))
	for _, report := range reports {
		got = append(got, report.Name)
	}
	return got
}

func labels(limits []Limit) []string {
	got := make([]string, 0, len(limits))
	for _, limit := range limits {
		got = append(got, limit.Label)
	}
	return got
}
