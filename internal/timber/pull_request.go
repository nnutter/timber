package timber

import (
	"encoding/json/v2"
	"fmt"
	"strings"
)

const pullRequestListLimit = 100

type pullRequestCheck struct {
	Conclusion string `json:"conclusion"`
	Status     string `json:"status"`
}

type pullRequestEntry struct {
	Number            int                `json:"number"`
	HeadRefName       string             `json:"headRefName"`
	StatusCheckRollup []pullRequestCheck `json:"statusCheckRollup"`
}

// parsePullRequestList maps branch names to their dashboard display string,
// such as "#56 ✓". Branches without an open pull request are absent.
func parsePullRequestList(output []byte) (map[string]string, error) {
	var entries []pullRequestEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("decode gh pull request list: %w", err)
	}

	pullRequests := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.HeadRefName == "" || entry.Number <= 0 {
			continue
		}
		if _, exists := pullRequests[entry.HeadRefName]; exists {
			continue
		}
		pullRequests[entry.HeadRefName] = fmt.Sprintf("#%d%s", entry.Number, formatPullRequestMark(entry.StatusCheckRollup))
	}
	return pullRequests, nil
}

func formatPullRequestMark(checks []pullRequestCheck) string {
	if len(checks) == 0 {
		return ""
	}
	pending := false
	for _, check := range checks {
		if strings.EqualFold(check.Conclusion, "FAILURE") {
			return " ✗"
		}
		if !strings.EqualFold(check.Status, "COMPLETED") {
			pending = true
		}
	}
	if pending {
		return " …"
	}
	return " ✓"
}
