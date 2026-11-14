package quotas

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// Standing drops the limits that a used-up longer window makes moot.
// A limit whose window the service does not give stays.
func Standing(limits []Limit) []Limit {
	var full []Limit
	for _, limit := range limits {
		if limit.Percent >= 100 {
			full = append(full, limit)
		}
	}
	kept := make([]Limit, 0, len(limits))
	for _, limit := range limits {
		if covered(limit, full) {
			continue
		}
		kept = append(kept, limit)
	}
	return kept
}

func covered(limit Limit, full []Limit) bool {
	for _, other := range full {
		if other.Window > 0 && hasWindow(limit) && longerThan(other, limit) {
			return true
		}
	}
	return false
}

// Kept keeps the plans with something to say.
// A missing sign-in is left out. A failure, a limit, a balance, or Copilot's
// notes when it has no finite quota stay.
func Kept(reports []Report) []Report {
	kept := make([]Report, 0, len(reports))
	for _, report := range reports {
		if !report.OK {
			if !report.Missing {
				kept = append(kept, report)
			}
			continue
		}
		if keepsReport(report) {
			kept = append(kept, report)
		}
	}
	return kept
}

// Ordered returns the plans to use first.
// The plan that stands to waste the most of its longest timed window comes
// first: unused share divided by the share of that window still left. That is
// the subscription to spend before its reset throws the remainder away. A plan
// with no clock comes after those, least used first. A balance with no limits
// comes next, and a plan that failed comes last.
func Ordered(reports []Report, now time.Time) []Report {
	ordered := slices.Clone(Kept(reports))
	slices.SortStableFunc(ordered, func(left, right Report) int {
		leftGroup, leftKey := rank(left, now)
		rightGroup, rightKey := rank(right, now)
		if leftGroup != rightGroup {
			return cmp.Compare(leftGroup, rightGroup)
		}
		return cmp.Compare(leftKey, rightKey)
	})
	return ordered
}

func rank(report Report, now time.Time) (int, float64) {
	if !report.OK {
		return 3, 0
	}
	if wasted, ok := planWaste(report.Limits, now); ok {
		// Negated so the ascending order puts the most waste first.
		return 0, -wasted
	}
	limit, ok := longest(report.Limits)
	if !ok {
		return 2, 0
	}
	return 1, limit.Percent
}

func longest(limits []Limit) (Limit, bool) {
	var best Limit
	found := false
	bestSpan := time.Duration(-1)
	for _, limit := range limits {
		span := time.Duration(-1)
		if limit.Window > 0 {
			span = limit.Window
		}
		if !found || wider(span, bestSpan) {
			best = limit
			bestSpan = span
			found = true
		}
	}
	return best, found
}

// planWaste is the waste in the longest window that still has a reset clock.
func planWaste(limits []Limit, now time.Time) (float64, bool) {
	var best float64
	var bestSpan time.Duration
	found := false
	for _, limit := range limits {
		waste, ok := windowWaste(limit, now)
		if !ok {
			continue
		}
		if !found || wider(limit.Window, bestSpan) {
			best = waste
			bestSpan = limit.Window
			found = true
		}
	}
	return best, found
}

// windowWaste is unused share divided by the share of the window still left.
// 1 uses up exactly what is left by the reset. Above that wastes quota. Below
// it runs out early. A window whose reset has passed wastes whatever it has left.
func windowWaste(limit Limit, now time.Time) (float64, bool) {
	share, ok := shareOf(limit, now)
	if !ok {
		return 0, false
	}
	unused := (100 - limit.Percent) / 100
	left := (100 - share) / 100
	if left <= 0 {
		if unused > 0 {
			return math.Inf(1), true
		}
		return 0, true
	}
	return unused / left, true
}

func shareOf(limit Limit, now time.Time) (float64, bool) {
	if limit.Window <= 0 || limit.ResetsAt.IsZero() {
		return 0, false
	}
	span := float64(limit.Window)
	left := float64(limit.ResetsAt.Sub(now))
	if left < 0 {
		left = 0
	}
	if left > span {
		left = span
	}
	return 100 - (left/span)*100, true
}

func hasWindow(limit Limit) bool { return limit.Window > 0 }

func longerThan(left, right Limit) bool { return left.Window > right.Window }

func wider(span, best time.Duration) bool { return span > best }

func keepsReport(report Report) bool {
	if len(report.Limits) > 0 {
		return true
	}
	if report.Balance != nil {
		return true
	}
	return report.ID == "github-copilot" && hasNotes(report)
}

func hasNotes(report Report) bool { return len(report.Notes) > 0 }
