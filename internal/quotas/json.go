package quotas

import (
	"cmp"
	"encoding/json/v2"
	"time"
)

// Encode returns the snapshot as the JSON a quotas panel draws from.
// The panel reads each window, its reset, any balance, and any failure,
// and it rejects null where a window, a reset, or a credit balance is absent.
// It sorts the plans itself, so the services' order stays as they answered.
func Encode(snapshot Snapshot) ([]byte, error) {
	reports := make([]any, 0, len(snapshot.Reports))
	for _, report := range snapshot.Reports {
		reports = append(reports, reportValue(report))
	}
	return json.Marshal(reportsDocument{
		FetchedAt: fetchedAt(snapshot.FetchedAt),
		Reports:   reports,
	})
}

// The panel treats a missing field as absent and rejects null for a window,
// a reset, or a credit balance, so those zero values stay off the wire.
type reportsDocument struct {
	FetchedAt *string `json:"fetchedAt"`
	Reports   []any   `json:"reports"`
}

type usageDocument struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	OK      bool             `json:"ok"`
	Limits  []limitDocument  `json:"limits"`
	Notes   []string         `json:"notes"`
	Balance *balanceDocument `json:"balance,omitzero"`
}

type failureDocument struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Error   string `json:"error"`
	Missing bool   `json:"missing,omitzero"`
}

type limitDocument struct {
	Label            string   `json:"label"`
	Percent          float64  `json:"percent"`
	ResetsAt         string   `json:"resetsAt,omitempty"`
	Level            string   `json:"level"`
	Binding          bool     `json:"binding,omitzero"`
	WindowMs         int64    `json:"windowMs,omitzero"`
	RemainingCredits *float64 `json:"remainingCredits,omitzero"`
}

type balanceDocument struct {
	Currency string  `json:"currency"`
	Total    float64 `json:"total"`
	Granted  float64 `json:"granted"`
	ToppedUp float64 `json:"toppedUp"`
	Enough   bool    `json:"enough"`
}

func reportValue(report Report) any {
	if !report.OK {
		return failureDocument{
			ID:      report.ID,
			Name:    report.Name,
			Error:   report.Error,
			Missing: report.Missing,
		}
	}
	return usageDocument{
		ID:      report.ID,
		Name:    report.Name,
		OK:      true,
		Limits:  limitValues(report.Limits),
		Notes:   notes(report.Notes),
		Balance: balanceValue(report.Balance),
	}
}

func limitValues(limits []Limit) []limitDocument {
	out := make([]limitDocument, 0, len(limits))
	for _, limit := range limits {
		out = append(out, limitValue(limit))
	}
	return out
}

func limitValue(limit Limit) limitDocument {
	documented := limitDocument{
		Label:    limit.Label,
		Percent:  limit.Percent,
		Level:    cmp.Or(string(limit.Level), string(LevelOK)),
		Binding:  limit.Binding,
		WindowMs: limit.Window.Milliseconds(),
	}
	if !limit.ResetsAt.IsZero() {
		documented.ResetsAt = limit.ResetsAt.UTC().Format(time.RFC3339)
	}
	if limit.HasRemainingCredits {
		documented.RemainingCredits = new(limit.RemainingCredits)
	}
	return documented
}

func notes(items []string) []string {
	if items == nil {
		return make([]string, 0)
	}
	return items
}

func balanceValue(balance *Balance) *balanceDocument {
	if balance == nil {
		return nil
	}
	return new(balanceDocument{
		Currency: balance.Currency,
		Total:    balance.Total,
		Granted:  balance.Granted,
		ToppedUp: balance.ToppedUp,
		Enough:   balance.Enough,
	})
}

func fetchedAt(at time.Time) *string {
	if at.IsZero() {
		return nil
	}
	return new(at.UTC().Format(time.RFC3339))
}
