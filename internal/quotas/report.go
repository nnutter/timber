// Package quotas reads AI subscription limits and shows which plan to use next.
package quotas

import "time"

// Level is how full a limit is, from the service's own word or from the percent.
type Level string

const (
	// LevelOK is a limit with room left.
	LevelOK Level = "ok"
	// LevelWarning is a limit that is filling up.
	LevelWarning Level = "warning"
	// LevelCritical is a limit that is nearly or fully used.
	LevelCritical Level = "critical"
)

// Limit is one window of a plan: the share used and when it resets.
type Limit struct {
	Label               string
	Percent             float64
	ResetsAt            time.Time
	Level               Level
	Binding             bool
	Window              time.Duration
	RemainingCredits    float64
	HasRemainingCredits bool
}

// Balance is money left on a prepaid account.
type Balance struct {
	Currency string
	Total    float64
	Granted  float64
	ToppedUp float64
	Enough   bool
}

// Report is one service's answer, or why it could not answer.
type Report struct {
	ID      string
	Name    string
	OK      bool
	Missing bool
	Error   string
	Limits  []Limit
	Notes   []string
	Balance *Balance
}

// Snapshot is every service's answer and when it was read.
type Snapshot struct {
	FetchedAt time.Time
	Reports   []Report
}

// MissingError means this machine has no sign-in for the service.
// The report leaves that service out.
type MissingError struct {
	Text string
}

func (e MissingError) Error() string { return e.Text }

// SaidError is a failure whose words are safe to show on the service's card.
type SaidError struct {
	Text string
}

func (e SaidError) Error() string { return e.Text }

// StatusError is a service's HTTP status, kept so a provider can name a special one.
type StatusError struct {
	Status int
	SaidError
}
