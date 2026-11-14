package quotas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicReadsSessionAndWeek(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/oauth/usage", r.URL.Path)
		assert.Equal(t, "Bearer sk-ant-oat-test", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{
			"limits": [
				{"kind": "session", "percent": 12, "resets_at": "2026-03-01T17:00:00Z", "is_active": true},
				{"kind": "weekly_all", "percent": 100, "resets_at": "2026-03-08T00:00:00Z"},
				{"kind": "weekly_scoped", "percent": 4, "scope": {"model": {"display_name": "Opus"}}}
			],
			"extra_usage": {"is_enabled": false}
		}`))
	})
	fetched, err := fetchAnthropic(t.Context(), envSource(map[string]string{
		"ANTHROPIC_TOKEN": "sk-ant-oat-test",
	}), client)
	require.NoError(t, err)
	report, ok := guard(anthropicID, anthropicName, fetched, err)

	require.True(t, ok)
	assert.Equal(t, anthropicName, report.Name)
	assert.Equal(t, []string{"Week, all models", "Week, Opus"}, labels(report.Limits))
	assert.Equal(t, []string{"Extra usage is off."}, report.Notes)
}

func TestCopilotTurnsRemainingIntoUsedAndSkipsAZeroReset(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token gh-test", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{
			"token_based_billing": true,
			"quota_reset_date": "2026-04-01T00:00:00Z",
			"quota_snapshots": {
				"premium_interactions": {"percent_remaining": 60, "quota_reset_at": 0, "remaining": 250},
				"chat": {"unlimited": true}
			}
		}`))
	})
	report, err := fetchCopilot(t.Context(), envSource(map[string]string{"GITHUB_TOKEN": "gh-test"}), client)
	require.NoError(t, err)
	require.Len(t, report.Limits, 1)
	assert.Equal(t, "Month", report.Limits[0].Label)
	assert.InDelta(t, 40.0, report.Limits[0].Percent, 1e-9)
	assert.True(t, report.Limits[0].HasRemainingCredits)
	assert.InDelta(t, 250.0, report.Limits[0].RemainingCredits, 1e-9)
	assert.Equal(t, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), report.Limits[0].ResetsAt)
}

func TestOmitOpenCodeRollingKeepsOtherFiveHourWindows(t *testing.T) {
	t.Parallel()
	snapshot := Snapshot{Reports: []Report{
		{ID: openCodeID, Name: openCodeName, OK: true, Limits: []Limit{
			{Label: "5 hours", Percent: 10},
			{Label: "Month", Percent: 20},
		}},
		{ID: miniMaxID, Name: miniMaxName, OK: true, Limits: []Limit{
			{Label: "5 hours", Percent: 30},
		}},
	}}

	got := OmitOpenCodeRolling(snapshot)

	require.Len(t, got.Reports, 2)
	assert.Equal(t, []string{"Month"}, labels(got.Reports[0].Limits))
	assert.Equal(t, []string{"5 hours"}, labels(got.Reports[1].Limits))
	assert.Equal(t, []string{"5 hours", "Month"}, labels(snapshot.Reports[0].Limits))
}

func TestOpenCodeLeavesARollingWindowWithoutAReset(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":10,"status":"ok"},"weekly":{"percent":80,"status":"warning","resetsAt":"2026-03-08T00:00:00Z"}}}`))
	})
	report, err := fetchOpenCode(t.Context(), envSource(map[string]string{"OPENCODE_GO_API_KEY": "oc-test"}), client)
	require.NoError(t, err)
	require.Len(t, report.Limits, 2)
	assert.Zero(t, report.Limits[0].ResetsAt)
	assert.Equal(t, LevelWarning, report.Limits[1].Level)
	assert.NotZero(t, report.Limits[1].ResetsAt)
}

func TestDeepSeekReadsTheYuanBalance(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"is_available":false,"balance_infos":[{"currency":"USD","total_balance":"1"},{"currency":"CNY","total_balance":"8.50","granted_balance":"3","topped_up_balance":"5.50"}]}`))
	})
	report, err := fetchDeepSeek(t.Context(), envSource(map[string]string{"DEEPSEEK_API_KEY": "ds-test"}), client)
	require.NoError(t, err)
	require.NotNil(t, report.Balance)
	assert.Equal(t, "CNY", report.Balance.Currency)
	assert.InDelta(t, 8.5, report.Balance.Total, 1e-9)
	assert.False(t, report.Balance.Enough)
}

func TestOllamaMeasuresTheIncludedMonth(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"included":{"allowance_usd":"20","balance_usd":"5","period":{"from":"2026-03-01T00:00:00Z","until":"2026-04-01T00:00:00Z"}},"purchased":{"balance_usd":"1"}}`))
	})
	report, err := fetchOllama(t.Context(), envSource(map[string]string{"OLLAMA_API_KEY": "ol-test"}), client)
	require.NoError(t, err)
	require.Len(t, report.Limits, 1)
	assert.InDelta(t, 75.0, report.Limits[0].Percent, 1e-9)
	assert.InDelta(t, 6.0, report.Balance.Total, 1e-9)
	assert.Contains(t, report.Notes[0], "rates")
}

func TestZAIRefusesAKeyWithoutACodingPlan(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"msg":"No Coding Plan subscription"}`))
	})
	_, err := fetchZAI(t.Context(), envSource(map[string]string{"ZAI_API_KEY": "zai-test"}), client)
	var said SaidError
	require.ErrorAs(t, err, &said)
	assert.Equal(t, "This key has no GLM Coding Plan: subscribe at z.ai/subscribe.", said.Error())
}

func TestGrokNamesATeamAccount(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	_, err := fetchGrok(t.Context(), grokSource("xai-token"), client)
	var said SaidError
	require.ErrorAs(t, err, &said)
	assert.Equal(t, "Grok keeps no personal quota for a team account.", said.Error())
}

func TestMiniMaxPutsTextFirstAndSkipsAnEmptyWeek(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"base_resp": {"status_code": 0},
			"model_remains": [{
				"model_name": "general",
				"start_time": 0,
				"end_time": 18000000,
				"current_interval_total_count": 100,
				"current_interval_usage_count": 80,
				"current_interval_remaining_percent": 20,
				"weekly_start_time": 0,
				"weekly_end_time": 604800000,
				"current_weekly_status": 3,
				"current_weekly_total_count": 0,
				"current_weekly_remaining_percent": 100
			}]
		}`))
	})
	report, err := fetchMiniMax(t.Context(), envSource(map[string]string{"MINIMAX_API_KEY": "mm-test"}), client)
	require.NoError(t, err)
	require.Len(t, report.Limits, 1)
	assert.Equal(t, "5 hours", report.Limits[0].Label)
	assert.InDelta(t, 80.0, report.Limits[0].Percent, 1e-9)
	assert.Contains(t, report.Notes, "Text has no weekly limit.")
}

func TestCodexReadsThePlanFromItsCLI(t *testing.T) {
	t.Parallel()
	script := writeScript(t, `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *'"id":0'*) printf '%s\n' '{"id":0,"result":{}}' ;;
    *'"id":1'*) printf '%s\n' '{"id":1,"result":{"rateLimits":{"planType":"plus","primary":{"usedPercent":22,"windowDurationMins":10080,"resetsAt":1700000000}}}}' ;;
  esac
done
`)
	report, err := fetchCodex(t.Context(), envSource(map[string]string{"QUOTAS_CODEX_COMMAND": script}), nil)
	require.NoError(t, err)
	assert.Equal(t, "ChatGPT", report.Name)
	require.Len(t, report.Limits, 1)
	assert.Equal(t, "Week", report.Limits[0].Label)
	assert.InDelta(t, 22.0, report.Limits[0].Percent, 1e-9)
}

func TestMuseReadsTheCLIUsage(t *testing.T) {
	t.Parallel()
	script := writeScript(t, `#!/bin/sh
printf '%s\n' '{"allowance":{"usedPercent":36,"windowDurationMins":300,"resetsAtMs":1700000000000,"tier":"pro"}}'
`)
	report, err := fetchMuse(t.Context(), envSource(map[string]string{"QUOTAS_MUSE_COMMAND": script}), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"5 hours"}, labels(report.Limits))
	assert.Empty(t, report.Notes)
}

func TestCollectorOmitsAMissingSignInAndReusesAFreshRead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var calls int
	collector := &Collector{
		now: func() time.Time { return now },
		fetchers: []fetcher{
			{id: "a", name: "Alpha", fetch: func(context.Context) (Report, error) {
				calls++
				return Report{Limits: []Limit{{Label: "Week", Percent: 10, Window: time.Hour, ResetsAt: now.Add(time.Hour)}}}, nil
			}},
			{id: "b", name: "Beta", fetch: func(context.Context) (Report, error) {
				return Report{}, MissingError{Text: "missing"}
			}},
		},
	}

	first, err := collector.Reports(t.Context(), true)
	require.NoError(t, err)
	second, err := collector.Reports(t.Context(), true)
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	require.Len(t, first.Reports, 1)
	assert.Equal(t, "Alpha", first.Reports[0].Name)
	assert.Equal(t, first, second)
}

func envSource(keys map[string]string) Source {
	return Source{
		Env:      func(key string) string { return keys[key] },
		Hermes:   func() string { return "" },
		UserHome: func() string { return "" },
		ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}
}

func grokSource(token string) Source {
	source := envSource(nil)
	source.ReadFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "auth.json" {
			return []byte(`{"providers":{"xai-oauth":{"tokens":{"access_token":"` + token + `"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}
	source.Hermes = func() string { return "/hermes" }
	return source
}

func stubClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	return &http.Client{Transport: hostRewrite{host: target.Host}}
}

type hostRewrite struct {
	host string
}

func (h hostRewrite) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = h.host
	clone.Host = h.host
	return http.DefaultTransport.RoundTrip(clone)
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o700))
	return path
}
