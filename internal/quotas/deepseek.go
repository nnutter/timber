package quotas

import (
	"context"
	"net/http"
)

const (
	deepSeekID   = "deepseek"
	deepSeekName = "DeepSeek"
	deepSeekURL  = "https://api.deepseek.com/user/balance"
)

func fetchDeepSeek(ctx context.Context, source Source, client *http.Client) (Report, error) {
	key, err := source.APIKey(deepSeekID, []string{"DEEPSEEK_API_KEY"}, "DeepSeek")
	if err != nil {
		return Report{}, err
	}
	decoded, err := getJSON(ctx, client, deepSeekURL, bearer(key), readWait)
	if err != nil {
		return Report{}, err
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, SaidError{Text: "It answered with something this plugin could not read."}
	}
	infos := asRecords(answer["balance_infos"])
	var main map[string]any
	for _, info := range infos {
		if info["currency"] == "CNY" {
			main = info
			break
		}
	}
	if main == nil {
		main = firstRecord(infos)
	}
	if main == nil {
		return Report{}, SaidError{Text: "The answer named no balance."}
	}
	total := percentOf(main["total_balance"])
	currency, _ := asText(main["currency"])
	enough := deepSeekEnough(answer, total)
	return Report{
		Name: deepSeekName,
		Balance: new(Balance{
			Currency: currency,
			Total:    total,
			Granted:  percentOf(main["granted_balance"]),
			ToppedUp: percentOf(main["topped_up_balance"]),
			Enough:   enough,
		}),
	}, nil
}

func deepSeekEnough(answer map[string]any, total float64) bool {
	if asBool(answer["is_available"]) {
		return true
	}
	if answer["is_available"] == false {
		return false
	}
	return aboveZero(total)
}
