package quotas

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLitFillsOneSquareForAnyUseAndAllSquaresOnlyWhenFull(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, Lit(0))
	assert.Equal(t, 1, Lit(0.1))
	assert.Equal(t, 6, Lit(32))
	assert.Equal(t, 19, Lit(99))
	assert.Equal(t, 20, Lit(100))
}

func TestUntilReset(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		reset time.Time
		want  string
		ok    bool
	}{
		{name: "absent"},
		{name: "now", reset: now.Add(20 * time.Second), want: "now", ok: true},
		{name: "minutes", reset: now.Add(12 * time.Minute), want: "in 12m", ok: true},
		{name: "hours and minutes", reset: now.Add(3*time.Hour + 10*time.Minute), want: "in 3h 10m", ok: true},
		{name: "days and hours", reset: now.Add(2*24*time.Hour + 4*time.Hour), want: "in 2d 4h", ok: true},
		{name: "days", reset: now.Add(48 * time.Hour), want: "in 2d", ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := UntilReset(tc.reset, now)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResetLabelDropsTheLeadingIn(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	label, ok := ResetLabel(now.Add(2*24*time.Hour+4*time.Hour), now)
	assert.True(t, ok)
	assert.Equal(t, "2d 4h", label)
}

func TestAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	assert.Empty(t, Age(time.Time{}, now))
	assert.Equal(t, "<1m ago", Age(now.Add(-10*time.Second), now))
	assert.Equal(t, "<1m ago", Age(now.Add(-50*time.Second), now))
	assert.Equal(t, "1m ago", Age(now.Add(-time.Minute), now))
	assert.Equal(t, "3m ago", Age(now.Add(-3*time.Minute), now))
	assert.Equal(t, "2h ago", Age(now.Add(-2*time.Hour), now))
	assert.Equal(t, "2d ago", Age(now.Add(-49*time.Hour), now))
}

func TestWindowLabel(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "5 hours", WindowLabel(300))
	assert.Equal(t, "Week", WindowLabel(10_080))
	assert.Equal(t, "Day", WindowLabel(1440))
	assert.Equal(t, "30 days", WindowLabel(43_200))
	assert.Equal(t, "Limit", WindowLabel(0))
}

func TestMoney(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "$1.23", Money(1.23, "USD"))
	assert.Equal(t, "¥8.50", Money(8.5, "CNY"))
	assert.Equal(t, "¥0.00", Money(0, ""))
	assert.Equal(t, "JPY 10.00", Money(10, "JPY"))
}

func TestOffPeakNoteUsesUTCWeekdayHours(t *testing.T) {
	t.Parallel()
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	assert.Contains(t, OffPeakNote(saturday), "Off-peak rates until ")

	weekdayPeak := time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)
	assert.Contains(t, OffPeakNote(weekdayPeak), "Full price until ")
	assert.Contains(t, OffPeakNote(weekdayPeak), "off-peak rates after.")
}
