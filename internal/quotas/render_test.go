package quotas

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

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
	assert.Contains(t, view, "1m ago")
	assert.Contains(t, view, "■")
	assert.Contains(t, view, "□")
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
