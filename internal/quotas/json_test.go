package quotas

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeKeepsTheServiceOrderAndTheClocks(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	// Sooner wastes less of its hour than Later wastes of its week, so the
	// screen would list Later first. A used-up five-hour window would also
	// hide the five-minute window. The panel sorts and hides for itself.
	sooner := Report{
		ID:   "sooner",
		Name: "Sooner",
		OK:   true,
		Limits: []Limit{
			{
				Label: "Hour", Percent: 90, Level: LevelWarning,
				Window: time.Hour, ResetsAt: now.Add(time.Hour),
			},
			{Label: "Open", Percent: 0},
			{
				Label: "Credits", Percent: 12.5, Level: LevelCritical,
				RemainingCredits: 0, HasRemainingCredits: true,
			},
		},
		Balance: &Balance{Currency: "USD", Total: 1.5, Granted: 1, ToppedUp: 0, Enough: false},
	}
	later := Report{
		ID:    "later",
		Name:  "Later",
		OK:    true,
		Notes: []string{"spare"},
		Limits: []Limit{
			{
				Label: "Week", Percent: 10, Binding: true,
				Window: 7 * 24 * time.Hour, ResetsAt: now.Add(7 * 24 * time.Hour),
			},
			{
				Label: "5 hours", Percent: 100, Level: LevelCritical,
				Window: 5 * time.Hour, ResetsAt: now.Add(5 * time.Hour),
			},
			{
				Label: "5 minutes", Percent: 40,
				Window: 5 * time.Minute, ResetsAt: now.Add(5 * time.Minute),
			},
		},
	}
	snapshot := Snapshot{FetchedAt: now, Reports: []Report{
		sooner,
		{ID: "gone", Name: "Gone", Missing: true, Error: "not signed in"},
		later,
		{ID: "down", Name: "Down", Error: "offline"},
	}}

	encoded, err := Encode(snapshot)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"fetchedAt": "2026-03-01T12:00:00Z",
		"reports": [
			{
				"id": "sooner",
				"name": "Sooner",
				"ok": true,
				"limits": [
					{
						"label": "Hour",
						"percent": 90,
						"resetsAt": "2026-03-01T13:00:00Z",
						"level": "warning",
						"windowMs": 3600000
					},
					{"label": "Open", "percent": 0, "level": "ok"},
					{
						"label": "Credits",
						"percent": 12.5,
						"level": "critical",
						"remainingCredits": 0
					}
				],
				"notes": [],
				"balance": {
					"currency": "USD",
					"total": 1.5,
					"granted": 1,
					"toppedUp": 0,
					"enough": false
				}
			},
			{
				"id": "gone",
				"name": "Gone",
				"ok": false,
				"error": "not signed in",
				"missing": true
			},
			{
				"id": "later",
				"name": "Later",
				"ok": true,
				"limits": [
					{
						"label": "Week",
						"percent": 10,
						"resetsAt": "2026-03-08T12:00:00Z",
						"level": "ok",
						"binding": true,
						"windowMs": 604800000
					},
					{
						"label": "5 hours",
						"percent": 100,
						"resetsAt": "2026-03-01T17:00:00Z",
						"level": "critical",
						"windowMs": 18000000
					},
					{
						"label": "5 minutes",
						"percent": 40,
						"resetsAt": "2026-03-01T12:05:00Z",
						"level": "ok",
						"windowMs": 300000
					}
				],
				"notes": ["spare"]
			},
			{"id": "down", "name": "Down", "ok": false, "error": "offline"}
		]
	}`, string(encoded))

	empty, err := Encode(Snapshot{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"fetchedAt": null, "reports": []}`, string(empty))
}
