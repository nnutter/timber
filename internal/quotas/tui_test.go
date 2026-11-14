package quotas

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScreenShowsHowOldTheReadingIs(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	opened, _ := model.Update(fetchedMsg{
		at: now,
		snapshot: Snapshot{
			FetchedAt: now.Add(-time.Minute),
			Reports:   []Report{{Name: "ChatGPT", OK: true, Limits: []Limit{{Label: "Week", Percent: 10}}}},
		},
	})

	assert.Contains(t, ansi.Strip(opened.View()), "1m ago")
}

func TestScreenScrollsWithTheWheelAndADrag(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})
	limits := make([]Limit, 0, 12)
	for index := range 12 {
		limits = append(limits, Limit{
			Label: "Window", Percent: float64(index), Window: time.Hour, ResetsAt: now.Add(time.Hour),
		})
	}

	sized, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 6})
	opened, _ := sized.Update(fetchedMsg{
		at: now,
		snapshot: Snapshot{
			FetchedAt: now,
			Reports:   []Report{{Name: "Plan", OK: true, Limits: limits}},
		},
	})
	before := ansi.Strip(opened.View())
	wheeled, _ := opened.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	afterWheel := ansi.Strip(wheeled.View())
	assert.NotEqual(t, before, afterWheel)
	assert.Contains(t, afterWheel, "q quit")

	pressed, _ := wheeled.Update(tea.MouseMsg{Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	dragged, _ := pressed.Update(tea.MouseMsg{Y: 8, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	afterDrag := ansi.Strip(dragged.View())
	assert.NotEqual(t, afterWheel, afterDrag)
	assert.Contains(t, afterDrag, "q quit")
}

func TestScreenSaysItIsReadingUntilTheFirstAnswer(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	view := model.View()

	assert.Contains(t, view, "Reading usage…")
	assert.Contains(t, view, "q quit")
}

func TestScreenQuitsOnTheQuitKeys(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyEsc},
	} {
		_, command := model.Update(key)
		require.NotNil(t, command)
		_, quit := command().(tea.QuitMsg)
		assert.True(t, quit, key.String())
	}
}

func TestScreenShowsAFailedRead(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	next, _ := model.Update(fetchedMsg{err: errors.New("offline"), at: time.Now()})

	view := ansi.Strip(next.View())
	assert.Contains(t, view, "offline")
	assert.NotContains(t, view, "Reading usage…")
}

func TestScreenRefreshReadsTheServicesAgain(t *testing.T) {
	t.Parallel()
	var fresh []bool
	model := newScreen(func(force bool) (Snapshot, error) {
		fresh = append(fresh, force)
		return Snapshot{}, nil
	})
	opened, _ := model.Update(fetchedMsg{at: time.Now(), snapshot: Snapshot{}})

	reading, command := opened.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	require.NotNil(t, command)
	pressed, ok := command().(fetchedMsg)
	require.True(t, ok)
	settled, _ := reading.Update(pressed)
	shown, ok := settled.(*screen)
	require.True(t, ok)

	_, command = shown.Update(refreshMsg{id: shown.pollID})
	require.NotNil(t, command)
	_, ok = command().(fetchedMsg)
	require.True(t, ok)
	assert.Equal(t, []bool{true, true}, fresh)
}

func TestScreenShowsThePollChoices(t *testing.T) {
	t.Parallel()
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	assert.Contains(t, ansi.Strip(model.View()), "r refresh · 1 1m · [5 5m] · 0 10m · q quit")

	one := press(t, model, '1')
	assert.Contains(t, ansi.Strip(one.View()), "[1 1m]")
	assert.NotContains(t, ansi.Strip(one.View()), "[5 5m]")

	ten := press(t, one, '0')
	assert.Contains(t, ansi.Strip(ten.View()), "[0 10m]")

	five := press(t, ten, '5')
	assert.Contains(t, ansi.Strip(five.View()), "[5 5m]")
	assert.NotContains(t, ansi.Strip(five.View()), "[0 10m]")
}

func TestScreenReadsAgainOnTheChosenInterval(t *testing.T) {
	t.Parallel()
	var reads int
	model := newScreen(func(bool) (Snapshot, error) {
		reads++
		return Snapshot{}, nil
	})
	opened, _ := model.Update(fetchedMsg{at: time.Now(), snapshot: Snapshot{}})
	current, ok := opened.(*screen)
	require.True(t, ok)
	require.Equal(t, 5*time.Minute, current.interval)

	ignored, command := opened.Update(refreshMsg{})
	assert.Nil(t, command)
	reading, command := ignored.Update(refreshMsg{id: current.pollID})
	require.NotNil(t, command)
	message, ok := command().(fetchedMsg)
	require.True(t, ok)

	settled, _ := reading.Update(message)
	pending, ok := settled.(*screen)
	require.True(t, ok)
	pendingID := pending.pollID
	chosen, command := settled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	require.NotNil(t, command)
	next, ok := chosen.(*screen)
	require.True(t, ok)
	assert.Equal(t, time.Minute, next.interval)

	stale, command := chosen.Update(refreshMsg{id: pendingID})
	assert.Nil(t, command)
	assert.Equal(t, 1, reads)
	_, command = stale.Update(refreshMsg{id: next.pollID})
	require.NotNil(t, command)
	_, ok = command().(fetchedMsg)
	require.True(t, ok)
	assert.Equal(t, 2, reads)
}

func press(t *testing.T, model tea.Model, key rune) tea.Model {
	t.Helper()
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	return next
}

func TestScreenMovesThePlanToSpendAsTheWindowRunsDown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	// Short wastes less of its ten minutes than Long wastes of its hour.
	// Once Short's reset passes, the remainder would be thrown away, so it
	// becomes the plan to spend.
	short := Report{Name: "Short", OK: true, Limits: []Limit{{
		Label: "Ten minutes", Percent: 50, Window: 10 * time.Minute, ResetsAt: now.Add(9 * time.Minute),
	}}}
	long := Report{Name: "Long", OK: true, Limits: []Limit{{
		Label: "Hour", Percent: 20, Window: time.Hour, ResetsAt: now.Add(30 * time.Minute),
	}}}
	model := newScreen(func(bool) (Snapshot, error) {
		return Snapshot{}, nil
	})

	opened, _ := model.Update(fetchedMsg{
		at:       now,
		snapshot: Snapshot{FetchedAt: now, Reports: []Report{short, long}},
	})
	first := ansi.Strip(opened.View())
	longAt := strings.Index(first, "Long")
	shortAt := strings.Index(first, "Short")
	require.NotEqual(t, -1, longAt)
	require.NotEqual(t, -1, shortAt)
	assert.Less(t, longAt, shortAt)

	later, _ := opened.Update(ageMsg(now.Add(9 * time.Minute)))
	second := ansi.Strip(later.View())
	longAt = strings.Index(second, "Long")
	shortAt = strings.Index(second, "Short")
	require.NotEqual(t, -1, longAt)
	require.NotEqual(t, -1, shortAt)
	assert.Less(t, shortAt, longAt)
}
