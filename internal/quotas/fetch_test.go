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
	reports := guard(anthropicID, anthropicName, []Report{fetched}, err)

	require.Len(t, reports, 1)
	report := reports[0]
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

func TestCopilotOmitsUnlimitedPremiumRequests(t *testing.T) {
	t.Parallel()
	client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"quota_snapshots":{"premium_interactions":{"unlimited":true,"overage_permitted":true}}}`))
	})
	report, err := fetchCopilot(t.Context(), envSource(map[string]string{"GITHUB_TOKEN": "gh-test"}), client)
	require.NoError(t, err)
	assert.Empty(t, report.Limits)
	assert.Empty(t, report.Notes)
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

func TestQuotaRefusalsNameCredentialSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fetch  func(context.Context, Source, *http.Client) (Report, error)
		source Source
		status int
		body   string
		want   string
	}{
		{"DeepSeek environment", fetchDeepSeek, envSource(map[string]string{"DEEPSEEK_API_KEY": "test-key"}), http.StatusUnauthorized, "", "DEEPSEEK_API_KEY environment variable"},
		{"MiniMax body refusal", fetchMiniMax, envSource(map[string]string{"MINIMAX_API_KEY": "test-key"}), http.StatusOK, `{"base_resp":{"status_code":1004}}`, "MINIMAX_API_KEY environment variable"},
		{"Copilot Pi sign-in", fetchCopilot, Source{
			UserHome: func() string { return "/test-home" },
			Env:      func(string) string { return "" },
			ReadFile: func(path string) ([]byte, error) {
				if path == "/test-home/.pi/agent/auth.json" {
					return []byte(`{"github-copilot":{"type":"oauth","access":"test-key"}}`), nil
				}
				return nil, os.ErrNotExist
			},
		}, http.StatusForbidden, "", "Pi"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := tc.fetch(t.Context(), tc.source, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "test-key")
		})
	}
}

func TestGrokNamesRejectedCredentialSource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		body string
	}{
		{"Grok CLI", ".grok/auth.json", `{"https://auth.x.ai::client":{"auth_mode":"oidc","key":"token"}}`},
		{"Pi", ".pi/agent/auth.json", `{"xai":{"type":"oauth","access":"token"}}`},
		{"Hermes", ".hermes/auth.json", `{"providers":{"xai-oauth":{"tokens":{"access_token":"token"}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, tc.path)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			source := Source{UserHome: func() string { return home }, Env: func(string) string { return "" }}
			client := stubClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
			_, err := fetchGrok(t.Context(), source, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "401")
			assert.Contains(t, err.Error(), tc.name)
		})
	}
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
	assert.NotContains(t, report.Notes, "Text has no weekly limit.")
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

func TestGoogleReadsQuotaGroupsIntoSeparateReports(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 19, 16, 0, 0, 0, time.UTC)
	script := writeScript(t, `#!/bin/sh
if [ "$1" != "-p" ] || [ "$2" != "/usage" ] || [ "$3" != "--output-format" ] || [ "$4" != "json" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
cat <<'EOF'
{
  "status": "SUCCESS",
  "command": {
    "name": "usage",
    "data": {
      "groups": [
        {
          "name": "Gemini Models",
          "description": "Models within this group: Gemini 4 Neon, Gemini 3.8 Flash",
          "buckets": [
            {"id": "gemini-weekly", "name": "Weekly Limit Remaining", "window": "weekly", "remaining_fraction": 0.9, "reset_time": "2026-11-26T14:00:00Z"},
            {"id": "gemini-5h", "name": "Five Hour Limit Remaining", "window": "5h", "remaining_fraction": 0.65, "reset_time": "2026-11-19T19:00:00Z"}
          ]
        },
        {
          "name": "Claude and GPT models",
          "description": "Models within this group: Claude Opus 5.5, GPT-OSS 120B",
          "buckets": [
            {"id": "3p-weekly", "name": "Weekly Limit Remaining", "window": "weekly", "reset_time": "2026-11-26T16:00:00Z"},
            {"id": "3p-5h", "name": "Five Hour Limit Remaining", "window": "5h", "remaining_fraction": 0.8, "reset_time": "2026-11-19T21:00:00Z"}
          ]
        },
        {
          "name": "Gemini 4 Argon",
          "description": "For paid Pro plans, Argon quota is separate.",
          "buckets": [
            {"id": "gemini-argon-wtu-weekly", "name": "Gemini 4 Argon Weekly Limit", "window": "weekly", "reset_time": "2026-11-20T16:00:00Z"}
          ]
        },
        {
          "name": "Empty",
          "buckets": []
        }
      ]
    }
  }
}
EOF
`)
	source := envSource(map[string]string{"QUOTAS_AGY_COMMAND": script})
	collector := &Collector{
		now: func() time.Time { return now },
		fetchers: []fetcher{{
			id:   googleID,
			name: googleName,
			fetch: func(ctx context.Context) ([]Report, error) {
				return fetchGoogle(ctx, source, nil)
			},
		}},
	}

	snapshot, err := collector.Reports(t.Context(), true)
	require.NoError(t, err)
	require.Len(t, snapshot.Reports, 2)

	gemini := snapshot.Reports[0]
	assert.Equal(t, "google-ai-gemini-models", gemini.ID)
	assert.Equal(t, "Google AI (Gemini Models)", gemini.Name)
	assert.Equal(t, []string{"5 hours", "Week"}, labels(gemini.Limits))
	assert.InDelta(t, 35.0, gemini.Limits[0].Percent, 1e-9)
	assert.InDelta(t, 10.0, gemini.Limits[1].Percent, 1e-9)
	assert.Equal(t, []string{"Models: Gemini 4 Neon, Gemini 3.8 Flash"}, gemini.Notes)

	argon := snapshot.Reports[1]
	assert.Equal(t, "google-ai-gemini-4-argon", argon.ID)
	assert.Equal(t, "Google AI (Gemini 4 Argon)", argon.Name)
	assert.Equal(t, []string{"Week"}, labels(argon.Limits))
	// A reset time without remaining_fraction represents an exhausted window.
	assert.InDelta(t, 100.0, argon.Limits[0].Percent, 1e-9)
	assert.Equal(t, LevelCritical, argon.Limits[0].Level)
	assert.Empty(t, argon.Notes)

	assert.Equal(t, []string{
		"Google AI (Gemini Models)",
		"Google AI (Gemini 4 Argon)",
	}, names(Ordered(snapshot.Reports, now)))
}

func TestGoogleCancelsAuthPromptAsMissing(t *testing.T) {
	t.Parallel()
	script := writeScript(t, `#!/bin/sh
echo "Authentication required. Please visit the URL to log in:" >&2
sleep 30
`)
	start := time.Now()
	_, err := fetchGoogle(t.Context(), envSource(map[string]string{"QUOTAS_AGY_COMMAND": script}), nil)
	var missing MissingError
	require.ErrorAs(t, err, &missing)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestGoogleHandlesCLIExitAndErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		script      string
		wantMissing bool
	}{
		{
			name:        "exit code 2 API key mode",
			script:      "#!/bin/sh\nexit 2\n",
			wantMissing: true,
		},
		{
			name:        "auth error payload",
			script:      "#!/bin/sh\nprintf '%s\n' '{\"status\":\"ERROR\",\"error\":\"authentication failed or timed out\"}'\n",
			wantMissing: true,
		},
		{
			name:   "general CLI failure",
			script: "#!/bin/sh\nexit 1\n",
		},
		{
			name:   "malformed JSON",
			script: "#!/bin/sh\nprintf 'not-json'\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script := writeScript(t, tc.script)
			_, err := fetchGoogle(t.Context(), envSource(map[string]string{"QUOTAS_AGY_COMMAND": script}), nil)
			if tc.wantMissing {
				var missing MissingError
				require.ErrorAs(t, err, &missing)
				return
			}
			var said SaidError
			require.ErrorAs(t, err, &said)
		})
	}
}

func TestCollectorOmitsAMissingSignInAndReusesAFreshRead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var calls int
	collector := &Collector{
		now: func() time.Time { return now },
		fetchers: []fetcher{
			singleFetcher("a", "Alpha", func(context.Context) (Report, error) {
				calls++
				return Report{Limits: []Limit{{Label: "Week", Percent: 10, Window: time.Hour, ResetsAt: now.Add(time.Hour)}}}, nil
			}),
			singleFetcher("b", "Beta", func(context.Context) (Report, error) {
				return Report{}, MissingError{Text: "missing"}
			}),
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
