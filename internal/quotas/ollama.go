package quotas

import (
	"context"
	"net/http"
	"time"
)

const (
	ollamaID   = "ollama"
	ollamaName = "Ollama Cloud"
	ollamaURL  = "https://ollama.com/api/balance"
)

func fetchOllama(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey("ollama-cloud", []string{"OLLAMA_API_KEY"}, "Ollama Cloud")
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, ollamaURL, bearer(key), readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something this plugin could not read."}
	}
	included, _ := asObject(answer["included"])
	purchased, _ := asObject(answer["purchased"])
	allowance := percentOf(included["allowance_usd"])
	granted := percentOf(included["balance_usd"])
	toppedUp := percentOf(purchased["balance_usd"])
	var limits []Limit
	if allowance > 0 {
		percent := min(100, max(0, (allowance-granted)/allowance*100))
		period, _ := asObject(included["period"])
		start := resetAt(period["from"])
		end := resetAt(period["until"])
		var minutes float64
		if !start.IsZero() && !end.IsZero() {
			minutes = end.Sub(start).Minutes()
		}
		limits = append(limits, Limit{
			Label:    "Month",
			Percent:  percent,
			ResetsAt: end,
			Level:    LevelOf(percent, ""),
			Window:   WindowDuration(minutes),
		})
	}
	total := granted + toppedUp
	return Report{
		Name:   ollamaName,
		Limits: limits,
		Notes:  []string{OffPeakNote(time.Now().UTC())},
		Balance: new(Balance{
			Currency: "USD",
			Total:    total,
			Granted:  granted,
			ToppedUp: toppedUp,
			Enough:   total > 0,
		}),
	}, nil
}
