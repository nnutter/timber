package cli

// listJSONRecord contains the fields asserted from `list --json`.
type listJSONRecord struct {
	Name        string `json:"name"`
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Upstream    string `json:"upstream"`
	Commit      string `json:"commit"`
	Clean       bool   `json:"clean"`
	StatusError bool   `json:"statusError"`
	TodoDone    int    `json:"todoDone"`
	TodoTotal   int    `json:"todoTotal"`
}
