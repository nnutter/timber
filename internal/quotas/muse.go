package quotas

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"

	json "encoding/json/v2"
)

const (
	museID      = "muse"
	museName    = "Muse Code"
	museMissing = "The Muse CLI is not installed: install it, then run muse login."
)

func fetchMuse(ctx context.Context, source Source, _ *http.Client) (Report, error) {
	command, err := source.Command("QUOTAS_MUSE_COMMAND", "muse", museMissing)
	if err != nil {
		return Report{}, err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, readWait, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))})
	defer cancel()
	process := exec.CommandContext(ctx, command, "usage", "--json")
	process.Dir = source.userHome()
	output, err := process.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Report{}, SaidError{Text: fmt.Sprintf("No answer in %d s.", int(readWait.Seconds()))}
		}
		return Report{}, MissingError{Text: museMissing}
	}
	var decoded any
	if err := json.Unmarshal(output, &decoded); err != nil {
		return Report{}, MissingError{Text: "The Muse CLI did not answer with its usage: run muse login, then try again."}
	}
	answer, ok := asObject(decoded)
	if !ok {
		return Report{}, MissingError{Text: "The Muse CLI did not answer with its usage: run muse login, then try again."}
	}
	allowance, ok := asObject(answer["allowance"])
	if !ok {
		return Report{Name: museName, Notes: []string{"No usage observed yet: Muse reports it after the first use in a window."}}, nil
	}
	percent := percentOf(allowance["usedPercent"])
	duration, durationOK := asNumber(allowance["windowDurationMins"])
	label := "5 hours"
	minutes := 300.0
	if durationOK {
		label = WindowLabel(duration)
		minutes = duration
	}
	limits := []Limit{{
		Label:    label,
		Percent:  percent,
		ResetsAt: resetAtMillis(allowance["resetsAtMs"]),
		Level:    LevelOf(percent, ""),
		Window:   WindowDuration(minutes),
	}}
	if weekly, ok := asObject(allowance["weekly"]); ok {
		weekPercent := percentOf(weekly["usedPercent"])
		limits = append(limits, Limit{
			Label:    "Week",
			Percent:  weekPercent,
			ResetsAt: resetAtMillis(weekly["resetsAtMs"]),
			Level:    LevelOf(weekPercent, ""),
			Window:   weekWindow,
		})
	}
	return Report{Name: museName, Limits: limits}, nil
}
